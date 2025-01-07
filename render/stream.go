package render

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/soockee/bass-kata/audio"
	"golang.org/x/sys/windows"
)

func RenderFromStream(subscription <-chan *audio.Subscription, deviceName string, ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "CaptureWithStream"))

	if err := renderStream(subscription, deviceName, ctx, wca.AUDCLNT_SHAREMODE_SHARED); err != nil {
		return fmt.Errorf("rendering failed: %w", err)
	}

	slog.Debug("Rendering completed successfully")
	return nil
}

func renderStream(subscription <-chan *audio.Subscription, deviceName string, ctx context.Context, mode uint32) error {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		return fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()

	ac, err := audio.SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("failed to get mix format: %w", err)
	}
	slog.Debug("Mix format", slog.Any("wfx", wfx))

	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	props := wca.AudioClientProperties{
		CbSize:                uint32(unsafe.Sizeof(wca.AudioClientProperties{})),
		BIsOffload:            0,
		AUDIO_STREAM_CATEGORY: wca.AudioCategory_Other,
		AUDCLNT_STREAMOPTIONS: wca.AUDCLNT_STREAMOPTIONS_MATCH_FORMAT,
	}

	if err := ac.SetClientProperties(&props); err != nil {
		return fmt.Errorf("failed to set client properties: %w", err)
	}

	closestMatch := (&wca.WAVEFORMATEX{})
	if err := ac.IsFormatSupported(mode, wfx, &closestMatch); err != nil {
		return fmt.Errorf("failed to check if format is supported: %w", err)
	}

	slog.Debug("Closest match", slog.Any("closestMatch", closestMatch))

	// Display audio format info
	slog.Debug("Render", slog.Any("WFormatTag", wfx.WFormatTag), slog.Any("WBitsPerSample", wfx.WBitsPerSample), slog.Any("Hz", wfx.NSamplesPerSec), slog.Any("Channels", wfx.NChannels))

	var defaultPeriodInFrames, fundamentalPeriodInFrames, minPeriodInFrames, maxPeriodInFrames uint32
	if err = ac.GetSharedModeEnginePeriod(wfx, &defaultPeriodInFrames, &fundamentalPeriodInFrames, &minPeriodInFrames, &maxPeriodInFrames); err != nil {
		return err
	}

	slog.Debug("Render", slog.Any("Default period in frames", defaultPeriodInFrames))
	slog.Debug("Render", slog.Any("Fundamental period in frames: ", fundamentalPeriodInFrames))
	slog.Debug("Render", slog.Any("Min period in frames: ", minPeriodInFrames))
	slog.Debug("Render", slog.Any("Max period in frames: ", maxPeriodInFrames))

	var latency time.Duration = time.Duration(float64(minPeriodInFrames)/float64(wfx.NSamplesPerSec)*1000) * time.Millisecond

	// 0 or AUDCLNT_STREAMFLAGS_EVENTCALLBACK or AUDCLNT_SESSIONFLAGS_XXX Constants
	if err = ac.InitializeSharedAudioStream(0, minPeriodInFrames, wfx, nil); err != nil {
		return err
	}

	var bufferFrames uint32
	if err := ac.GetBufferSize(&bufferFrames); err != nil {
		return err
	}
	fmt.Printf("Allocated buffer size: %d\n", bufferFrames)

	var arc *wca.IAudioRenderClient
	if err := ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		return err
	}
	defer arc.Release()

	// Start audio rendering
	if err := ac.Start(); err != nil {
		return fmt.Errorf("failed to start audio client: %w", err)
	}
	defer ac.Stop()

	slog.Debug("Start rendering with timer driven mode", slog.Any("Mode", mode))

	queue := make(chan *audio.Subscription, 200) // Buffered channel for queue
	var wg sync.WaitGroup

	// Producer: Reads from subscription and queues events
	j := 0

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				slog.Debug("Context cancelled in producer")
				close(queue)
				return
			case event, ok := <-subscription:
				if !ok { // subscription channel is closed
					close(queue) // Ensure queue is closed when subscription ends
					return
				}
				select {
				case queue <- event:
					slog.Debug("Enqueued event", slog.Any("Count", j))
					j++
				default:
					slog.Warn("Queue full, dropping event")
				}
			}
		}
	}()
	// Consumer: Processes events from the queue
	wg.Add(1)
	i := 0
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done(): // Stop processing when context is canceled
				slog.Debug("Context cancelled, stopping rendering")
				return
			case event, ok := <-queue: // Process events from the queue
				if !ok {
					slog.Debug("Queue closed, exiting")
					return
				}
				slog.Debug("Processing event", slog.Int("Count", i))
				i++
				processEvent(event, ac, arc, bufferFrames, latency, wfx.NBlockAlign, ctx)
			}
		}
	}()

	wg.Wait()
	return nil
}

func processEvent(e *audio.Subscription, ac *wca.IAudioClient3, arc *wca.IAudioRenderClient, bufferFrames uint32, latency time.Duration, blockAlign uint16, ctx context.Context) {
	var data *byte
	var padding uint32
	var offset int
	if e.Position.From-e.Position.To == 0 {
		return
	}

	streamData := e.Stream.Read(*e.Position)

	for e.Position.From+offset < e.Position.To {
		select {
		case <-ctx.Done():
			slog.Debug("Context cancelled, stopping processing")
			return
		default:
			// Handle the audio rendering logic
			var frames uint32
			// Wait for frames to be ready
			for {
				if err := ac.GetCurrentPadding(&padding); err != nil {
					slog.Error("Failed to get current padding", slog.Any("error", err))
					continue
				}
				frames = bufferFrames - padding
				if frames <= 0 {
					select {
					case <-ctx.Done():
						slog.Debug("Context cancelled during waiting")
						return
					case <-time.After(latency): // Avoid busy-waiting
						continue
					}
				}
				// Get render buffer
				if err := arc.GetBuffer(frames, &data); err != nil {
					slog.Error("Failed to get render buffer", slog.Any("error", err))
					continue
				}
				break
			}

			lim := int(frames) * int(blockAlign)
			from := e.Position.From + offset
			to := e.Position.From + offset + lim
			if len(streamData) < to {
				to = len(streamData)
			}
			if len(streamData) < from {
				from = 0
			}
			b := streamData[from:to]
			copyToRenderBuffer(data, b)
			offset += lim

			// Release buffer
			if err := arc.ReleaseBuffer(frames, 0); err != nil {
				slog.Error("Failed to release buffer", slog.Any("error", err))
				return
			}
		}
	}
}

func copyToRenderBuffer(dst *byte, src []byte) {
	dstBytes := unsafe.Slice((*byte)(dst), len(src))
	copy(dstBytes, src)
}
