package capture

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/go-ole/go-ole"

	"github.com/moutend/go-wca/pkg/wca"
	"github.com/soockee/bass-kata/devices"
	"golang.org/x/sys/windows"
)

func Capture(filename string, ctx context.Context, deviceName string) error {
	// Prepare the output WAV file

	outputFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Perform the capture operation
	samples, waveFmt, err := captureSharedTimerDriven(ctx, deviceName)
	if err != nil {
		return fmt.Errorf("audio capture failed: %w", err)
	}

	// Read and parse the `.wav` file
	err = wave.WriteFrames(samples, *waveFmt, filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	slog.Info("Audio capture successful", slog.String("file", filename))
	return nil
}

func captureSharedTimerDriven(ctx context.Context, deviceName string) ([]wave.Frame, *wave.WaveFmt, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "captureSharedTimerDriven"))

	// Initialize COM library
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()

	// Get default capture audio endpoint
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return nil, nil, fmt.Errorf("failed to create device enumerator: %w", err)
	}
	defer mmde.Release()

	// Find the desired device by name
	device, err := devices.FindDeviceByName(mmde, deviceName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find device by name: %w", err)
	}
	defer device.Device.Release()

	var ac *wca.IAudioClient
	if err := device.Device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return nil, nil, fmt.Errorf("failed to activate audio client: %w", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return nil, nil, fmt.Errorf("failed to get mix format: %w", err)
	}

	wfx.WFormatTag = 1
	wfx.NBlockAlign = (wfx.WBitsPerSample / 8) * wfx.NChannels
	wfx.NAvgBytesPerSec = wfx.NSamplesPerSec * uint32(wfx.NBlockAlign)
	wfx.CbSize = 0

	waveFmt := wave.NewWaveFmt(int(wfx.WFormatTag), int(wfx.NChannels), int(wfx.NSamplesPerSec), int(wfx.WBitsPerSample), nil)

	// Configure buffer size and latency
	var defaultPeriod, minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return nil, nil, fmt.Errorf("failed to get device period: %w", err)
	}

	// Display audio format info
	slog.Info("--------")
	slog.Info("Capture")
	slog.Info("Format", slog.Any("PCM_bit_signed_integer", wfx.WBitsPerSample))
	slog.Info("Rate", slog.Any("Hz", wfx.NSamplesPerSec))
	slog.Info("Channels", slog.Any("Channels", wfx.NChannels))
	slog.Info("--------")

	latency := time.Duration(int(minimumPeriod) * 100)

	// Initialize audio client in shared mode
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, minimumPeriod, 0, wfx, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize audio client: %w", err)
	}

	// Start audio capture
	acc, err := startCapture(ac)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start audio capture: %w", err)
	}
	defer acc.Release()

	// Capture loop
	frames, err := captureLoop(ctx, acc, waveFmt, latency)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to capture audio: %w", err)
	}

	slog.Info("Audio capture completed")
	return frames, &waveFmt, nil
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

	slog.Info("Audio capture started")
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
			slog.Info("Capture cancelled")
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

// type aliases for conversion functions
type (
	bytesToIntF func([]byte) int
)
type channelSelect int

var (
	// max value depending on the bit size
	maxValues = map[int]int{
		8:  math.MaxInt8,
		16: math.MaxInt16,
		32: math.MaxInt32,
		64: math.MaxInt64,
	}
	// figure out which 'to int' function to use..
	byteSizeToIntFunc = map[int]bytesToIntF{
		16: bits16ToInt,
		32: bits32ToInt,
	}
	MonoLeft  channelSelect = 1
	MonoRight channelSelect = 2
	Stereo    channelSelect = 3
)

// turn a 16-bit byte array into an int
func bits16ToInt(b []byte) int {
	if len(b) != 2 {
		panic("Expected size 4!")
	}
	var payload int16
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &payload)
	if err != nil {
		// TODO: make safe
		panic(err)
	}
	return int(payload) // easier to work with ints
}

// turn a 32-bit byte array into an int
func bits32ToInt(b []byte) int {
	if len(b) != 4 {
		panic("Expected size 4!")
	}
	var payload int32
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &payload)
	if err != nil {
		// TODO: make safe
		panic(err)
	}
	return int(payload) // easier to work with ints
}

// parseRawData takes raw audio data and converts it to a slice of audio frames
func parseRawData(wfmt wave.WaveFmt, rawdata []byte, channel channelSelect) []wave.Frame {
	bytesSampleSize := wfmt.BitsPerSample / 8
	frames := []wave.Frame{}

	if channel == Stereo {
		for i := 0; i < len(rawdata); i += bytesSampleSize {
			rawFrame := rawdata[i : i+bytesSampleSize]
			frameInt := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame)
			scaled := scaleFrame(frameInt, wfmt.BitsPerSample)
			frames = append(frames, scaled)
		}
		return frames
	}

	if channel == MonoLeft {
		for i := 0; i < len(rawdata); i += bytesSampleSize * 2 {
			rawFrame1 := rawdata[i : i+bytesSampleSize]
			// rawFrame2 := rawdata[i+bytesSampleSize : i+bytesSampleSize*2]
			frameInt1 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame1)
			// frameInt2 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame2)
			scaled1 := scaleFrame(frameInt1, wfmt.BitsPerSample)
			// scaled2 := scaleFrame(frameInt2, wfmt.BitsPerSample)

			frames = append(frames, scaled1, scaled1) // duplicate the frame
		}
	}
	if channel == MonoRight {
		for i := 0; i < len(rawdata); i += bytesSampleSize * 2 {
			// rawFrame1 := rawdata[i : i+bytesSampleSize]
			rawFrame2 := rawdata[i+bytesSampleSize : i+bytesSampleSize*2]
			// frameInt1 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame1)
			frameInt2 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame2)
			// scaled1 := scaleFrame(frameInt1, wfmt.BitsPerSample)
			scaled2 := scaleFrame(frameInt2, wfmt.BitsPerSample)

			frames = append(frames, scaled2, scaled2) // duplicate the frame
		}
	}

	return frames
}

// scaleFrame scales an unscaled integer to a float64
func scaleFrame(unscaled, bits int) wave.Frame {
	maxV := maxValues[bits]
	return wave.Frame(float64(unscaled) / float64(maxV))
}
