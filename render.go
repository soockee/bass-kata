package record

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"

	"github.com/moutend/go-wca/pkg/wca"
)

func Render(stream *AudioStream, deviceName string, ctx context.Context) error {
	ac, err := SetupAudioClient(deviceName)
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
		AUDIO_STREAM_CATEGORY: wca.AudioCategory_Other,
	}

	if err := ac.SetClientProperties(&props); err != nil {
		return fmt.Errorf("failed to set client properties: %w", err)
	}

	closestMatch := (&wca.WAVEFORMATEX{})
	if err := ac.IsFormatSupported(wca.AUDCLNT_SHAREMODE_SHARED, wfx, &closestMatch); err != nil {
		return fmt.Errorf("failed to check if format is supported: %w", err)
	}

	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return fmt.Errorf("failed to get device period: %w", err)
	}

	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in shared mode
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return fmt.Errorf("failed to initialize audio client: %w", err)
	}

	var bufferFrames uint32
	if err := ac.GetBufferSize(&bufferFrames); err != nil {
		return err
	}

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

	<-stream.Ready()
	loop(stream, ac, arc, latency, ctx, bufferFrames, wfx.NBlockAlign)
	return nil
}

func loop(stream *AudioStream, ac *wca.IAudioClient2, arc *wca.IAudioRenderClient, latency time.Duration, ctx context.Context, bufferFrames uint32, blockAlign uint16) error {
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
			streamData := stream.Read()
			if len(streamData) == 0 {
				continue
			}

			if err := ac.GetCurrentPadding(&padding); err != nil {
				slog.Error("Failed to get current padding", slog.Any("error", err))
				return err
			}
			frames := bufferFrames - padding
			if frames <= 0 {
				time.Sleep(latency) // Avoid busy-waiting
				continue
			}

			// Get render buffer
			if err := arc.GetBuffer(frames, &data); err != nil {
				slog.Error("Failed to get render buffer", slog.Any("error", err))
				return err
			}

			// Optimize buffer copy
			lim := int(frames) * int(blockAlign)
			buflen := cap(streamData) - offset
			if buflen < lim {
				lim = buflen
			}
			copyToRenderBuffer(data, streamData[offset:offset+lim])
			offset += lim

			// Release buffer
			if err := arc.ReleaseBuffer(frames, 0); err != nil {
				slog.Error("Failed to release buffer", slog.Any("error", err))
				return err
			}
		}
	}
	return nil
}

func copyToRenderBuffer(dst *byte, src []byte) {
	dstBytes := unsafe.Slice((*byte)(dst), len(src))
	copy(dstBytes, src)
}
