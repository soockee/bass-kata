package capture

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/go-ole/go-ole"
	"github.com/soockee/bass-kata/audio"
	"golang.org/x/sys/windows"

	"github.com/moutend/go-wca/pkg/wca"
)

func CaptureToFile(deviceName string, filename string, ctx context.Context) error {
	// TODO: Is LockOSThread necessary?
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err != nil {
		return fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "captureSharedTimerDriven"))

	// Prepare the output WAV file
	outputFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

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

	// wfx.WFormatTag = 1
	// wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	// wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	// wfx.CbSize = 0

	waveFmt := wave.NewWaveFmt(int(wfx.WFormatTag), int(wfx.NChannels), int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), nil)

	op := &audio.AudioClientOpt{
		DeviceName: deviceName,
		WaveFmt:    waveFmt,
		Wfx:        wfx,
		Ctx:        ctx,
	}

	stream := audio.NewAudioStream()
	stream.SetFmt(op.WaveFmt)
	<-stream.Ready()

	// Perform the capture operation
	err = captureSharedTimerDriven(stream, ac, op)
	if err != nil {
		return fmt.Errorf("audio capture failed: %w", err)
	}
	<-stream.Done()

	data := stream.Read()

	frames := audio.TransfromRawData(op.WaveFmt, data, audio.MonoRight)

	// Read and parse the `.wav` file
	err = wave.WriteFrames(frames, waveFmt, filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	slog.Debug("Audio capture successful", slog.String("file", filename))
	return nil
}

// CaptureWithStream captures audio and exposes it as a stream
func CaptureWithStream(stream *audio.AudioStream, deviceName string, ctx context.Context) error {

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
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("failed to get mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	wfx.WFormatTag = 1
	wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	wfx.CbSize = 0

	op := &audio.AudioClientOpt{
		DeviceName: deviceName,
		Wfx:        wfx,
		WaveFmt:    wave.NewWaveFmt(int(wfx.WFormatTag), int(wfx.NChannels), int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), nil),
		Ctx:        ctx,
		Mode:       wca.AUDCLNT_SHAREMODE_SHARED,
	}

	stream.SetFmt(op.WaveFmt)

	err = captureSharedTimerDriven(stream, ac, op)
	if err != nil {
		return err
	}

	return nil
}

func captureSharedTimerDriven(stream *audio.AudioStream, ac *wca.IAudioClient, op *audio.AudioClientOpt) error {

	// Configure buffer size and latency
	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return fmt.Errorf("failed to get device period: %w", err)
	}

	// Display audio format info
	slog.Debug("--------")
	slog.Debug("Capture")
	slog.Debug("Format", slog.Any("PCM_bit_signed_integer", op.Wfx.WBitsPerSample))
	slog.Debug("Rate", slog.Any("Hz", op.Wfx.NSamplesPerSec))
	slog.Debug("Channels", slog.Any("Channels", op.Wfx.NChannels))
	slog.Debug("--------")

	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in shared mode
	if err := ac.Initialize(op.Mode, 0, minimumPeriod, 0, op.Wfx, nil); err != nil {
		return fmt.Errorf("failed to initialize audio client: %w", err)
	}

	// Start audio capture
	acc, err := startCapture(ac)
	if err != nil {
		return fmt.Errorf("failed to start audio capture: %w", err)
	}
	defer acc.Release()

	// Capture loop
	err = captureLoop(stream, acc, latency, op)

	if err != nil {
		return fmt.Errorf("failed to capture audio: %w", err)
	}

	slog.Debug("Audio capture completed")
	return nil
}

func startCapture(ac *wca.IAudioClient) (*wca.IAudioCaptureClient, error) {
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

func captureLoop(stream *audio.AudioStream, acc *wca.IAudioCaptureClient, latency time.Duration, op *audio.AudioClientOpt) error {
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
			time.Sleep(latency / 2)
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

			stream.Write(buf)
		}
	}

	stream.Close()

	return nil
}
