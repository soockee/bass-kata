package capture

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
	"unsafe"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/soockee/bass-kata/audio"
	"golang.org/x/sys/windows"
)

// CaptureWithStream captures audio and exposes it as a stream
func CaptureDevice(mux *audio.AudioMux, deviceName string, ctx context.Context) error {

	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		return fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()
	// TODO: Is LockOSThread necessary?
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "CaptureWithStream"))

	ac, err := audio.SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX

	// Mix format" wfx="&{WFormatTag:65534 NChannels:2 NSamplesPerSec:48000 NAvgBytesPerSec:384000 NBlockAlign:8 WBitsPerSample:32 CbSize:22}"
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("failed to get mix format: %w", err)
	}
	const bitsPerSample = 32
	const channelCount = 2
	const nBlockAlign = channelCount * bitsPerSample / 8
	const sampleRate = 48000
	wfx.WFormatTag = audio.WAVE_FORMAT_EXTENSIBLE
	wfx.NChannels = uint16(2)
	wfx.NSamplesPerSec = uint32(sampleRate)
	wfx.NAvgBytesPerSec = uint32(sampleRate * nBlockAlign)
	wfx.NBlockAlign = uint16(nBlockAlign)
	wfx.WBitsPerSample = uint16(bitsPerSample)
	wfx.CbSize = uint16(0x16)

	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	op := &audio.AudioClientOpt{
		DeviceName: deviceName,
		Wfx:        wfx,
		WaveFmt:    wave.NewWaveFmt(int(wfx.WFormatTag), int(wfx.NChannels), int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), nil),
		Ctx:        ctx,
		Mode:       wca.AUDCLNT_SHAREMODE_SHARED,
	}

	mux.Stream.SetFmt(op.WaveFmt)

	err = captureSharedTimerDriven(mux, ac, op)
	if err != nil {
		return err
	}

	return nil
}

func captureSharedTimerDriven(mux *audio.AudioMux, ac *wca.IAudioClient3, op *audio.AudioClientOpt) error {
	var defaultPeriodInFrames, fundamentalPeriodInFrames, minPeriodInFrames, maxPeriodInFrames uint32
	if err := ac.GetSharedModeEnginePeriod(op.Wfx, &defaultPeriodInFrames, &fundamentalPeriodInFrames, &minPeriodInFrames, &maxPeriodInFrames); err != nil {
		return err
	}

	slog.Debug("Capture", slog.Any("Default period in frames", defaultPeriodInFrames))
	slog.Debug("Capture", slog.Any("Fundamental period in frames: ", fundamentalPeriodInFrames))
	slog.Debug("Capture", slog.Any("Min period in frames: ", minPeriodInFrames))
	slog.Debug("Capture", slog.Any("Max period in frames: ", maxPeriodInFrames))

	var latency time.Duration = time.Duration(float64(minPeriodInFrames)/float64(op.Wfx.NSamplesPerSec)*1000) * time.Millisecond
	if err := ac.InitializeSharedAudioStream(wca.AUDCLNT_SHAREMODE_SHARED, minPeriodInFrames, op.Wfx, nil); err != nil {
		return err
	}

	acc, err := startCapture(ac)
	if err != nil {
		return fmt.Errorf("failed to start audio capture: %w", err)
	}
	defer acc.Release()

	err = captureLoop(mux, acc, latency, op)

	if err != nil {
		return fmt.Errorf("failed to capture audio: %w", err)
	}

	slog.Debug("Audio capture completed")
	return nil
}

func startCapture(ac *wca.IAudioClient3) (*wca.IAudioCaptureClient, error) {
	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return nil, fmt.Errorf("failed to get audio capture client: %w", err)
	}

	if err := ac.Start(); err != nil {
		acc.Release()
		return nil, fmt.Errorf("failed to start audio client: %w", err)
	}

	slog.Debug("Audio capture started")
	return acc, nil
}

func captureLoop(mux *audio.AudioMux, acc *wca.IAudioCaptureClient, latency time.Duration, op *audio.AudioClientOpt) error {
	var (
		isCapturing  = true
		framesToRead uint32
		data         *byte
		flags        uint32
		packetLength uint32
	)

	// wait a bit so the stream can be ready
	time.Sleep(latency * 10)

	for isCapturing {
		if err := acc.GetNextPacketSize(&packetLength); err != nil {
			continue
		}
		select {
		case <-op.Ctx.Done():
			slog.Debug("Capture cancelled")
			isCapturing = false
			continue
		default:
			// time.Sleep(latency / 2)
			if packetLength == 0 {
				continue
			}

			if err := acc.GetBuffer(&data, &framesToRead, &flags, nil, nil); err != nil {
				return fmt.Errorf("failed to get buffer: %w", err)
			}
			if framesToRead == 0 {
				continue
			}

			start := unsafe.Pointer(data)
			lim := int(framesToRead) * int(op.WaveFmt.BlockAlign)
			buf := make([]byte, lim)
			for n := 0; n < lim; n++ {
				buf[n] = *(*byte)(unsafe.Pointer(uintptr(start) + uintptr(n)))
			}

			if err := acc.ReleaseBuffer(framesToRead); err != nil {
				return fmt.Errorf("failed to release buffer: %w", err)
			}

			mux.Write(buf)
		}
	}

	mux.Close()

	return nil
}
