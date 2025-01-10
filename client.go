package record

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

type AudioClientOpt struct {
	DeviceName string
	WaveFmt    wave.WaveFmt
	Wfx        *wca.WAVEFORMATEX
	Ctx        context.Context
}

func SetupAudioClient(deviceName string) (*wca.IAudioClient2, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "captureSharedTimerDriven"))

	// Initialize COM library
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return nil, fmt.Errorf("failed to initialize COM: %w", err)
	}
	defer ole.CoUninitialize()

	// Get default capture audio endpoint
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return nil, fmt.Errorf("failed to create device enumerator: %w", err)
	}
	defer mmde.Release()

	// Find the desired device by name
	device, err := FindDeviceByName(mmde, deviceName, EAll, DEVICE_STATE_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("failed to find device by name: %w", err)
	}
	defer device.Device.Release()

	var ac *wca.IAudioClient2
	if err := device.Device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return nil, fmt.Errorf("failed to activate audio client: %w", err)
	}
	return ac, nil
}
