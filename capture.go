package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"

	"github.com/moutend/go-wca/pkg/wca"
)

// Capture captures audio and exposes it as a stream
func Capture(stream *AudioStream, deviceName string, ctx context.Context) error {
	ac, err := SetupAudioClient(deviceName)
	if err != nil {
		return fmt.Errorf("failed to setup audio client: %w", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("failed to get mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	// Configure buffer size and latency
	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return fmt.Errorf("failed to get device period: %w", err)
	}

	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in shared mode
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return fmt.Errorf("failed to initialize audio client: %w", err)
	}

	// Start audio capture
	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return fmt.Errorf("failed to get audio capture client: %w", err)
	}
	defer acc.Release()

	if err := ac.Start(); err != nil {
		return fmt.Errorf("failed to start audio client: %w", err)
	}

	stream.Start()
	if err := captureloop(stream, acc, latency, ctx, wfx.NBlockAlign); err != nil {
		return fmt.Errorf("failed to capture audio: %w", err)
	}

	return nil
}

func captureloop(stream *AudioStream, acc *wca.IAudioCaptureClient, latency time.Duration, ctx context.Context, blockAlign uint16) error {
	var (
		isCapturing  = true
		framesToRead uint32
		data         *byte
		flags        uint32
		packetLength uint32
	)

	for isCapturing {
		if err := acc.GetNextPacketSize(&packetLength); err != nil {
			continue
		}
		select {
		case <-ctx.Done():
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
			lim := int(framesToRead) * int(blockAlign)
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
