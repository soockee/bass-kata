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

	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

func Render(filename string, ctx context.Context) error {
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
	if err := renderSharedTimerDriven(ctx, &audio); err != nil {
		return fmt.Errorf("rendering failed: %w", err)
	}

	slog.Info("Rendering completed successfully")
	return nil
}

func renderSharedTimerDriven(ctx context.Context, audio *wave.Wave) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "renderSharedTimerDriven"))

	// Initialize COM library
	if err = ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return
	}
	defer ole.CoUninitialize()

	// Get default audio endpoint
	var de *wca.IMMDeviceEnumerator
	if err = wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		return
	}
	defer de.Release()

	var mmd *wca.IMMDevice
	if err = de.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &mmd); err != nil {
		return
	}
	defer mmd.Release()

	// Fetch device properties
	var ps *wca.IPropertyStore
	if err = mmd.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
		return
	}
	defer ps.Release()

	var pv wca.PROPVARIANT
	if err = ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
		return
	}
	slog.Info("Rendering audio", slog.String("To", pv.String()))

	// Activate audio client
	var ac *wca.IAudioClient
	if err = mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return
	}
	defer ac.Release()

	// Set up audio format
	var wfx *wca.WAVEFORMATEX
	if err = ac.GetMixFormat(&wfx); err != nil {
		return
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	wfx.WFormatTag = 1
	wfx.NSamplesPerSec = uint32(audio.SampleRate)
	wfx.WBitsPerSample = uint16(audio.BitsPerSample)
	wfx.NChannels = uint16(audio.NumChannels)
	wfx.NBlockAlign = uint16(audio.BlockAlign)
	wfx.NAvgBytesPerSec = uint32(audio.ByteRate)
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
	if err = ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return
	}
	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in shared mode
	if err = ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return
	}

	var arc *wca.IAudioRenderClient
	if err = ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		return
	}
	defer arc.Release()

	// Start audio rendering
	if err := ac.Start(); err != nil {
		return fmt.Errorf("failed to start audio client: %w", err)
	}
	defer ac.Stop()

	slog.Info("Start rendering with shared timer driven mode")

	// Rendering loop
	var (
		raw             = audio.RawData
		offset          int
		isPlaying       = true
		bufferFrameSize uint32
		data            *byte
		padding         uint32
	)

	if err := ac.GetBufferSize(&bufferFrameSize); err != nil {
		return fmt.Errorf("failed to get buffer size: %w", err)
	}

	for isPlaying {
		select {
		case <-ctx.Done():
			slog.Info("Rendering cancelled")
			isPlaying = false
			continue
		default:
			if offset >= audio.Subchunk2Size {
				isPlaying = false
				break
			}
			// Check the buffer availability
			if err := ac.GetCurrentPadding(&padding); err != nil {
				slog.Error("Failed to get current padding", slog.Any("error", err))
				continue
			}
			availableFrameSize := bufferFrameSize - padding
			if availableFrameSize == 0 {
				// Use a non-blocking wait
				select {
				case <-ctx.Done():
					slog.Info("Rendering cancelled during latency wait")
					return ctx.Err()
				case <-time.After(latency): // Wait for the latency duration
					continue
				}
			}
			// Get render buffer
			if err := arc.GetBuffer(availableFrameSize, &data); err != nil {
				slog.Error("Failed to get render buffer", slog.Any("error", err))
				continue
			}

			// Copy audio data to render buffer
			start := unsafe.Pointer(data)
			lim := int(availableFrameSize) * int(wfx.NBlockAlign)
			remaining := audio.Subchunk2Size - offset
			if remaining < lim {
				lim = remaining
			}
			for i := 0; i < lim; i++ {
				*(*byte)(unsafe.Pointer(uintptr(start) + uintptr(i))) = raw[offset+i]
			}
			offset += lim

			// Release buffer
			if err := arc.ReleaseBuffer(availableFrameSize, 0); err != nil {
				slog.Error("Failed to release buffer", slog.Any("error", err))
				return err
			}

			// Check context again for cancellation
			select {
			case <-ctx.Done():
				slog.Info("Rendering cancelled during buffer release")
				return ctx.Err()
			default:
				// slog.Debug("Rendering audio", slog.Any("offset", offset))
			}
		}
	}
	return nil
}
