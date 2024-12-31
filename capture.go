package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wav"
	"github.com/moutend/go-wca/pkg/wca"
)

func capture(filename string, ctx context.Context, cancel context.CancelFunc, duration time.Duration, deviceName string) (err error) {
	var audio *wav.File
	var outputFile *os.File
	var file []byte

	// Handle OS signals like Ctrl+C
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	go func() {
		select {
		case <-signalChan:
			slog.Info("Interrupted by SIGINT")
			cancel()
		}
		return
	}()

	// Prepare the output WAV file
	if outputFile, err = os.Create(filename); err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Perform the capture operation
	if audio, err = captureSharedTimerDriven(ctx, duration, deviceName); err != nil {
		return
	}
	if file, err = wav.Marshal(audio); err != nil {
		return
	}

	if err = os.WriteFile(filename, file, 0644); err != nil {
		return
	}

	slog.Info("Audio capture successful", slog.String("File", filename))
	return nil
}

func captureSharedTimerDriven(ctx context.Context, duration time.Duration, deviceName string) (audio *wav.File, err error) {
	// Initialize COM library
	if err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return
	}
	defer ole.CoUninitialize()

	// Get default capture audio endpoint
	var mmde *wca.IMMDeviceEnumerator
	if err = wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return
	}
	defer mmde.Release()

	devices := ListDevices(mmde)

	var mmd *wca.IMMDevice
	// Find the device by name
	for _, device := range devices {
		name := device.name.String()
		slog.Info("Device", slog.String("Name", name))
		if name == deviceName {
			mmd = device.device
			continue
		}
		device.device.Release()
	}

	// if err = mmde.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &mmd); err != nil {
	// 	return
	// }
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
	slog.Info("Capturing audio", slog.String("From", pv.String()))

	// Activate audio client
	var ac *wca.IAudioClient
	if err = mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return
	}
	defer ac.Release()

	// Get audio format
	var wfx *wca.WAVEFORMATEX
	if err = ac.GetMixFormat(&wfx); err != nil {
		return
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	wfx.WFormatTag = 1
	wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	wfx.CbSize = 0

	// Configure audio format and allocate buffer
	if audio, err = wav.New(int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), int(wfx.NChannels)); err != nil {
		return
	}

	// Display audio format info
	slog.Info("--------")
	slog.Info("Capture")
	slog.Info("Format", slog.Any("PCM_bit_signed_integer", wfx.WBitsPerSample))
	slog.Info("Rate", slog.Any("Hz", wfx.NSamplesPerSec))
	slog.Info("Channels", slog.Any("Channels", wfx.NChannels))
	slog.Info("--------")

	// Initialize audio client
	var defaultPeriod wca.REFERENCE_TIME
	var minimumPeriod wca.REFERENCE_TIME
	var latency time.Duration
	if err = ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return
	}
	latency = time.Duration(int(defaultPeriod) * 100)
	slog.Info("Default period", slog.Any("defaultPeriod", defaultPeriod))
	slog.Info("Minimum period", slog.Any("defaultPeriod", minimumPeriod))
	slog.Info("Latency: ", slog.Any("latency", latency))

	if err = ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, wca.REFERENCE_TIME(10000000*duration.Seconds()), 0, wfx, nil); err != nil {
		return
	}

	var bufferFrameSize uint32
	if err = ac.GetBufferSize(&bufferFrameSize); err != nil {
		return
	}
	slog.Info("Allocated buffer", slog.Any("size", bufferFrameSize))

	// Start capture client
	var acc *wca.IAudioCaptureClient
	if err = ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return
	}
	defer acc.Release()

	if err = ac.Start(); err != nil {
		return
	}
	slog.Info("Started capturing audio in shared mode")
	if duration <= 0 {
		slog.Info("Press Ctrl-C to stop capturing")
	}

	// Capture loop
	var output = []byte{}
	var offset int
	var isCapturing bool = true
	var currentDuration time.Duration
	var b *byte
	var data *byte
	var availableFrameSize uint32
	var flags uint32
	var devicePosition uint64
	var qcpPosition uint64

	time.Sleep(latency)

	time.Sleep(latency)

	for {
		if !isCapturing {
			break
		}
		select {
		case <-ctx.Done():
			isCapturing = false
			break
		default:
			// Wait for buffering.
			time.Sleep(latency / 2)

			currentDuration = time.Duration(float64(offset) / float64(wfx.WBitsPerSample/8) / float64(wfx.NChannels) / float64(wfx.NSamplesPerSec) * float64(time.Second))
			if duration != 0 && currentDuration > duration {
				isCapturing = false
				break
			}
			if err = acc.GetBuffer(&data, &availableFrameSize, &flags, &devicePosition, &qcpPosition); err != nil {
				continue
			}
			if availableFrameSize == 0 {
				continue
			}

			start := unsafe.Pointer(data)
			lim := int(availableFrameSize) * int(wfx.NBlockAlign)
			buf := make([]byte, lim)

			for n := 0; n < lim; n++ {
				b = (*byte)(unsafe.Pointer(uintptr(start) + uintptr(n)))
				buf[n] = *b
			}
			offset += lim
			output = append(output, buf...)

			if err = acc.ReleaseBuffer(availableFrameSize); err != nil {
				return
			}
		}
	}

	io.Copy(audio, bytes.NewBuffer(output))

	slog.Info("Stop capturing")
	if err = ac.Stop(); err != nil {
		return
	}
	return
}
