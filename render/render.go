package render

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
	"unsafe"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/go-ole/go-ole"
	"github.com/soockee/bass-kata/audio"

	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

func Render(filename string, deviceName string, ctx context.Context) error {
	// Ensure input file is provided
	if filename == "" {
		return fmt.Errorf("specify WAVE audio file (*.wav)")
	}

	// Read and parse the `.wav` file
	audio, err := wave.ReadWaveFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Render the audio in shared timer-driven mode
	if err := renderTimerDriven(ctx, deviceName, &audio, wca.AUDCLNT_SHAREMODE_EXCLUSIVE); err != nil {
		return fmt.Errorf("rendering failed: %w", err)
	}

	slog.Info("Rendering completed successfully")
	return nil
}

func RenderFromStream(stream *audio.AudioStream, deviceName string, ctx context.Context) error {
	// TODO: Is LockOSThread necessary?
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "CaptureWithStream"))

	if err := renderTimerDrivenStream(stream, deviceName, ctx, wca.AUDCLNT_SHAREMODE_EXCLUSIVE); err != nil {
		return fmt.Errorf("rendering failed: %w", err)
	}

	slog.Info("Rendering completed successfully")
	return nil
}

func renderTimerDrivenStream(stream *audio.AudioStream, deviceName string, ctx context.Context, mode uint32) error {
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
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	wfx.WFormatTag = 1
	wfx.NChannels = uint16(stream.Fmt.NumChannels)
	wfx.NSamplesPerSec = uint32(stream.Fmt.SampleRate)
	wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	wfx.CbSize = 0

	<-stream.Ready()

	// wfx.WBitsPerSample = uint16(stream.Fmt.BitsPerSample)
	// wfx.NBlockAlign = uint16(stream.Fmt.BlockAlign)
	// wfx.NAvgBytesPerSec = uint32(stream.Fmt.ByteRate)

	var resampler *audio.Resampler
	if !audio.CompareWaveFmtWfx(stream.Fmt, wfx) {
		slog.Info("WaveFmt and Wfx mismatch", slog.Any("WaveFmt", stream.Fmt), slog.Any("Wfx", wfx))
		resampler, err = audio.NewResampler(int(wfx.NChannels), stream.Fmt.SampleRate, int(wfx.NSamplesPerSec))
		if err != nil {
			return err
		}
	}

	if resampler != nil {
		slog.Info("Resampling audio", slog.Int("From", resampler.FromRate), slog.Int("To", resampler.ToRate))
	}

	// Display audio format info
	slog.Info("--------")
	slog.Info("Render")
	slog.Info("Format", slog.Any("PCM_bit_signed_integer", wfx.WBitsPerSample))
	slog.Info("Rate", slog.Any("Hz", wfx.NSamplesPerSec))
	slog.Info("Channels", slog.Any("Channels", wfx.NChannels))
	slog.Info("--------")

	var defaultPeriod wca.REFERENCE_TIME
	var minimumPeriod wca.REFERENCE_TIME
	var latency time.Duration
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return err
	}
	latency = time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client
	if err := ac.Initialize(mode, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return err
	}

	var bufferFrameSize uint32
	if err := ac.GetBufferSize(&bufferFrameSize); err != nil {
		return err
	}
	fmt.Printf("Allocated buffer size: %d\n", bufferFrameSize)

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

	slog.Info("Start rendering with timer driven mode", slog.Any("Mode", mode))

	// Rendering loop
	var (
		data        *byte
		padding     uint32
		isRendering = true
		offset      int
	)

	for isRendering {
		select {
		case <-ctx.Done():
			slog.Info("Rendering cancelled")
			isRendering = false
		case <-stream.Done():
			slog.Info("AudioStream closed")
			isRendering = false
		default:
			// Read data from the stream
			streamData := stream.Read()
			if len(streamData) == 0 {
				continue
			}

			// Batch operations to reduce cgo overhead
			if err := ac.GetCurrentPadding(&padding); err != nil {
				slog.Error("Failed to get current padding", slog.Any("error", err))
				continue
			}
			availableFrameSize := bufferFrameSize - padding
			if availableFrameSize <= 0 {
				time.Sleep(latency) // Avoid busy-waiting
				continue
			}

			// Get render buffer
			if err := arc.GetBuffer(availableFrameSize, &data); err != nil {
				slog.Error("Failed to get render buffer", slog.Any("error", err))
				continue
			}

			// Optimize buffer copy
			lim := min(len(streamData), int(availableFrameSize)*int(wfx.NBlockAlign))
			copyToRenderBuffer(data, streamData[offset:offset+lim])
			offset += lim

			// Release buffer
			if err := arc.ReleaseBuffer(availableFrameSize, 0); err != nil {
				slog.Error("Failed to release buffer", slog.Any("error", err))
				return err
			}
		}
	}
	time.Sleep(latency)
	return nil
}

func copyToRenderBuffer(dst *byte, src []byte) {
	dstBytes := unsafe.Slice((*byte)(dst), len(src))
	copy(dstBytes, src)
}

func renderTimerDriven(ctx context.Context, deviceName string, wavAudio *wave.Wave, mode uint32) error {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		return fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "renderSharedTimerDriven"))

	ac, err := audio.SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("failed to get mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	wfx.WFormatTag = 1
	wfx.NSamplesPerSec = uint32(wavAudio.SampleRate)
	wfx.WBitsPerSample = uint16(wavAudio.BitsPerSample)
	wfx.NChannels = uint16(wavAudio.NumChannels)
	wfx.NBlockAlign = uint16(wavAudio.BlockAlign)
	wfx.NAvgBytesPerSec = uint32(wavAudio.ByteRate)
	wfx.CbSize = 0

	// Display audio format info
	slog.Info("--------")
	slog.Info("Render")
	slog.Info("Format", slog.Any("PCM_bit_signed_integer", wfx.WBitsPerSample))
	slog.Info("Rate", slog.Any("Hz", wfx.NSamplesPerSec))
	slog.Info("Channels", slog.Any("Channels", wfx.NChannels))
	slog.Info("--------")

	// Configure buffer size and latency
	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return err
	}
	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in exclusive mode
	if err := ac.Initialize(mode, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return err
	}

	var arc *wca.IAudioRenderClient
	if err := ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		return err
	}
	defer arc.Release()

	var bufferFrameSize uint32
	if err := ac.GetBufferSize(&bufferFrameSize); err != nil {
		return fmt.Errorf("failed to get buffer size: %w", err)
	}

	slog.Info("Allocated buffer size", slog.Any("bufferFrameSize", bufferFrameSize))
	slog.Info("Latency: ", slog.Any("Latency", latency))
	slog.Info("--------")

	// Start audio rendering
	if err := ac.Start(); err != nil {
		return fmt.Errorf("failed to start audio client: %w", err)
	}
	defer ac.Stop()

	slog.Info("Start rendering with shared timer driven mode")

	// Rendering loop
	var (
		raw       = wavAudio.RawData
		offset    int
		isPlaying = true
		data      *byte
		padding   uint32
	)

	for isPlaying {
		select {
		case <-ctx.Done():
			slog.Info("Rendering cancelled")
			isPlaying = false
			continue
		default:
			if offset >= wavAudio.Subchunk2Size {
				isPlaying = false
				break
			}
			if err := ac.GetCurrentPadding(&padding); err != nil {
				slog.Error("Failed to get current padding", slog.Any("error", err))
				continue
			}
			availableFrameSize := bufferFrameSize - padding
			if availableFrameSize <= 0 {
				continue
			}

			// Get render buffer
			if err := arc.GetBuffer(availableFrameSize, &data); err != nil {
				slog.Error("Failed to get render buffer", slog.Any("error", err))
				continue
			}

			// Optimize buffer copy
			lim := int(availableFrameSize) * int(wfx.NBlockAlign)
			remaining := wavAudio.Subchunk2Size - offset
			if remaining < lim {
				lim = remaining
			}

			copyToRenderBuffer(data, raw[offset:offset+lim])
			offset += lim

			// Release buffer
			if err := arc.ReleaseBuffer(availableFrameSize, 0); err != nil {
				slog.Error("Failed to release buffer", slog.Any("error", err))
				return err
			}
			time.Sleep(latency / 2)
		}
	}
	// Render samples remaining in buffer.
	time.Sleep(latency)
	return nil
}
