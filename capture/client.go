package capture

import (
	"fmt"
	"log/slog"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/soockee/bass-kata/devices"
	"golang.org/x/sys/windows"
)

func SetupAudioClient(deviceName string) (*wca.IAudioClient, error) {
	// TODO: Is LockOSThread necessary?
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
	device, err := devices.FindDeviceByName(mmde, deviceName, devices.ECapture, devices.DEVICE_STATE_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("failed to find device by name: %w", err)
	}
	defer device.Device.Release()

	var ac *wca.IAudioClient
	if err := device.Device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return nil, fmt.Errorf("failed to activate audio client: %w", err)
	}
	return ac, nil
}
