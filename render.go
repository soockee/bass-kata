package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wav"
	"github.com/moutend/go-wca/pkg/wca"
)

var version = "latest"
var revision = "latest"

func render(filename string, ctx context.Context, cancel context.CancelFunc) (err error) {
	var filenameFlag FilenameFlag
	var versionFlag bool
	var audio = &wav.File{} // Represents the `.wav` file structure
	var file []byte

	filenameFlag.Value = filename

	// Display version info if requested
	if versionFlag {
		slog.Info("%s-%s\n", version, revision)
		return
	}

	// Ensure input file is provided
	if filenameFlag.Value == "" {
		return
	}

	// Read and parse the `.wav` file
	if file, err = os.ReadFile(filenameFlag.Value); err != nil {
		return
	}
	if err = wav.Unmarshal(file, audio); err != nil {
		return
	}

	// Handle OS signals like Ctrl+C
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Goroutine to handle interrupt signal
	go func() {
		select {
		case <-signalChan:
			slog.Info("Interrupted by SIGINT")
			cancel()
		}
		return
	}()

	// Render the audio in exclusive timer-driven mode
	if err = renderExclusiveTimerDriven(ctx, audio); err != nil {
		return
	}
	slog.Info("Successfully done")
	return
}

func renderExclusiveTimerDriven(ctx context.Context, audio *wav.File) (err error) {
	// Initialize COM library
	if err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
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
	wfx.NSamplesPerSec = uint32(audio.SamplesPerSec())
	wfx.WBitsPerSample = uint16(audio.BitsPerSample())
	wfx.NChannels = uint16(audio.Channels())
	wfx.NBlockAlign = uint16(audio.BlockAlign())
	wfx.NAvgBytesPerSec = uint32(audio.AvgBytesPerSec())
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

	// Initialize audio client in exclusive mode
	if err = ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return
	}

	// Fetch render buffer
	var bufferFrameSize uint32
	if err = ac.GetBufferSize(&bufferFrameSize); err != nil {
		return
	}
	slog.Info("Allocated buffer", slog.Any("size", bufferFrameSize))

	var arc *wca.IAudioRenderClient
	if err = ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		return
	}
	defer arc.Release()

	// Start audio rendering
	if err = ac.Start(); err != nil {
		return
	}

	slog.Info("Start rendering with exclusive timer driven mode")
	slog.Info("Press Ctrl-C to quit")

	// Rendering loop
	var input = audio.Bytes()
	var data *byte
	var offset int
	var padding, availableFrameSize uint32
	var isPlaying = true

	for isPlaying {
		select {
		case <-ctx.Done():
			isPlaying = false
		default:
			if offset >= audio.Length() {
				isPlaying = false
				break
			}
			if err = ac.GetCurrentPadding(&padding); err != nil {
				continue
			}
			availableFrameSize = bufferFrameSize - padding
			if availableFrameSize == 0 {
				continue
			}
			if err = arc.GetBuffer(availableFrameSize, &data); err != nil {
				continue
			}

			// Copy audio data to render buffer
			start := unsafe.Pointer(data)
			lim := int(availableFrameSize) * int(wfx.NBlockAlign)
			remaining := audio.Length() - offset
			if remaining < lim {
				lim = remaining
			}
			for n := 0; n < lim; n++ {
				*(*byte)(unsafe.Pointer(uintptr(start) + uintptr(n))) = input[offset+n]
			}
			offset += lim
			if err = arc.ReleaseBuffer(availableFrameSize, 0); err != nil {
				return
			}
			time.Sleep(latency / 2)
		}
	}

	// Flush remaining samples and stop
	time.Sleep(latency)
	return ac.Stop()
}
