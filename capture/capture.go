package capture

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
	"unsafe"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/soockee/bass-kata/audio"

	"github.com/moutend/go-wca/pkg/wca"
)

func CaptureToFile(deviceName string, filename string, ctx context.Context) error {
	// Prepare the output WAV file
	outputFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	ac, err := SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	wfx, err := GetDeviceWfx(ac)
	if err != nil {
		return fmt.Errorf("failed to get device wave format: %w", err)
	}

	waveFmt := wave.NewWaveFmt(int(wfx.WFormatTag), int(wfx.NChannels), int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), nil)

	op := &audio.AudioClientOpt{
		DeviceName: deviceName,
		WaveFmt:    waveFmt,
		Wfx:        wfx,
		Ctx:        ctx,
	}

	// Perform the capture operation
	samples, err := captureSharedTimerDriven(nil, ac, op)
	if err != nil {
		return fmt.Errorf("audio capture failed: %w", err)
	}

	// Read and parse the `.wav` file
	err = wave.WriteFrames(samples, waveFmt, filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	slog.Debug("Audio capture successful", slog.String("file", filename))
	return nil
}

// CaptureWithStream captures audio and exposes it as a stream
func CaptureWithStream(stream *audio.AudioStream, deviceName string, ctx context.Context) error {
	ac, err := SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	wfx, err := GetDeviceWfx(ac)
	if err != nil {
		return fmt.Errorf("failed to get device wave format: %w", err)
	}

	op := &audio.AudioClientOpt{
		DeviceName: deviceName,
		Wfx:        wfx,
		WaveFmt:    wave.NewWaveFmt(1, 2, 44100, 16, nil),
		Ctx:        ctx,
	}

	samples, err := captureSharedTimerDriven(stream, ac, op)
	if err != nil {
		return err
	}

	// Process final audio frames if needed (e.g., write to file)
	_ = samples // Placeholder for additional processing logic

	return nil
}

func GetDeviceWfx(ac *wca.IAudioClient) (*wca.WAVEFORMATEX, error) {
	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return wfx, fmt.Errorf("failed to get mix format: %w", err)
	}

	wfx.WFormatTag = 1
	wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	wfx.CbSize = 0

	return wfx, nil
}

func captureSharedTimerDriven(stream *audio.AudioStream, ac *wca.IAudioClient, op *audio.AudioClientOpt) ([]wave.Frame, error) {

	// Configure buffer size and latency
	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return nil, fmt.Errorf("failed to get device period: %w", err)
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
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, op.Wfx, nil); err != nil {
		return nil, fmt.Errorf("failed to initialize audio client: %w", err)
	}

	// Start audio capture
	acc, err := startCapture(ac)
	if err != nil {
		return nil, fmt.Errorf("failed to start audio capture: %w", err)
	}
	defer acc.Release()

	// Capture loop
	frames, err := captureLoop(op.Ctx, acc, op.WaveFmt, latency)

	if err != nil {
		return nil, fmt.Errorf("failed to capture audio: %w", err)
	}

	slog.Debug("Audio capture completed")
	return frames, nil
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

func captureLoop(ctx context.Context, acc *wca.IAudioCaptureClient, waveFmt wave.WaveFmt, latency time.Duration) ([]wave.Frame, error) {
	var (
		isCapturing  = true
		framesToRead uint32
		data         *byte
		flags        uint32
		output       = []byte{}
		offset       int
		packetLength uint32
	)

	for isCapturing {
		time.Sleep(latency)
		if err := acc.GetNextPacketSize(&packetLength); err != nil {
			continue
		}
		select {
		case <-ctx.Done():
			slog.Debug("Capture cancelled")
			isCapturing = false
			continue
		default:
			if packetLength == 0 {
				continue
			}

			if err := acc.GetBuffer(&data, &framesToRead, &flags, nil, nil); err != nil {
				return nil, fmt.Errorf("failed to get buffer: %w", err)
			}
			if framesToRead == 0 {
				continue
			}

			// check flags
			if flags&wca.AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY != 0 {
				slog.Warn("Data discontinuity detected")
			}

			if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
				slog.Warn("Silent frame detected")
			}

			if flags&wca.AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR != 0 {
				slog.Warn("Timestamp error detected")
			}

			start := unsafe.Pointer(data)
			lim := int(framesToRead) * int(waveFmt.BlockAlign)
			buf := make([]byte, lim)
			for n := 0; n < lim; n++ {
				buf[n] = *(*byte)(unsafe.Pointer(uintptr(start) + uintptr(n)))
			}
			offset += lim
			output = append(output, buf...)

			if err := acc.ReleaseBuffer(framesToRead); err != nil {
				return nil, fmt.Errorf("failed to release buffer: %w", err)
			}
		}
	}

	f := parseRawData(waveFmt, output, MonoRight)

	//f = audio.ApplyPan(f, audio.CalculateConstantPowerPosition(0.5))

	return f, nil
}
