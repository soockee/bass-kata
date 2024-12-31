package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func devices(ctx context.Context, cancel context.CancelFunc) error {
	quit := make(chan os.Signal, 1)   // Creates a channel to receive OS signals
	signal.Notify(quit, os.Interrupt) // Subscribes to interrupt signals (Ctrl+C)

	// Initializes COM library for use by the calling thread
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return err // Returns an error if initialization fails
	}
	defer ole.CoUninitialize() // Ensures COM is uninitialized when the function ends

	var mmde *wca.IMMDeviceEnumerator // Pointer to the audio device enumerator

	// Creates an instance of the audio device enumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return err
	}
	defer mmde.Release() // Releases the enumerator instance when done

	// Defines a set of callback functions for audio device events
	callback := wca.IMMNotificationClientCallback{
		OnDefaultDeviceChanged: onDefaultDeviceChanged, // Triggered when the default device changes
		OnDeviceAdded:          onDeviceAdded,          // Triggered when a device is added
		OnDeviceRemoved:        onDeviceRemoved,        // Triggered when a device is removed
		OnDeviceStateChanged:   onDeviceStateChanged,   // Triggered when a device state changes
		OnPropertyValueChanged: onPropertyValueChanged, // Triggered when a device property changes
	}

	// Creates a notification client with the defined callbacks
	mmnc := wca.NewIMMNotificationClient(callback)

	// Registers the notification client with the audio device enumerator
	if err := mmde.RegisterEndpointNotificationCallback(mmnc); err != nil {
		return err
	}

	// Main monitoring loop
	for {
		select {
		case <-ctx.Done(): // Exit gracefully if the context is canceled
			slog.Info("Context canceled, exiting devices monitoring.")
			return nil
		case <-quit: // Handle interrupt signal
			slog.Info("Received keyboard interrupt.")
			cancel() // Trigger cancellation
			return nil
		case <-time.After(5 * time.Minute): // Exit after a timeout
			slog.Info("Received timeout signal.")
			cancel() // Trigger cancellation
			return nil
		}
	}
}

func onDefaultDeviceChanged(flow wca.EDataFlow, role wca.ERole, pwstrDeviceId string) error {
	slog.Info("Called OnDefaultDeviceChanged", slog.Any("flow", flow), slog.Any("role", role), slog.Any("pwstrDeviceId", pwstrDeviceId))
	return nil
}

func onDeviceAdded(pwstrDeviceId string) error {
	slog.Info("Called OnDeviceAdded", slog.Any("pwstrDeviceId", pwstrDeviceId))

	return nil
}

func onDeviceRemoved(pwstrDeviceId string) error {
	slog.Info("Called OnDeviceRemoved", slog.Any("pwstrDeviceId", pwstrDeviceId))

	return nil
}

func onDeviceStateChanged(pwstrDeviceId string, dwNewState uint64) error {
	slog.Info("Called OnDeviceStateChanged", slog.Any("pwstrDeviceId", pwstrDeviceId), slog.Any("dwNewState", dwNewState))

	return nil
}

func onPropertyValueChanged(pwstrDeviceId string, key uint64) error {
	slog.Info("Called OnPropertyValueChanged", slog.Any("pwstrDeviceId", pwstrDeviceId), slog.Any("key", key))
	return nil
}

const (
	// EDataFlow enum
	eRender              uint32 = 0
	eCapture             uint32 = 1
	eAll                 uint32 = 2
	EDataFlow_enum_count uint32 = 3
	// DeviceState enum
	DEVICE_STATE_ACTIVE     uint32 = 0x00000001
	DEVICE_STATE_DISABLED   uint32 = 0x00000002
	DEVICE_STATE_NOTPRESENT uint32 = 0x00000004
	DEVICE_STATE_UNPLUGGED  uint32 = 0x00000008
	DEVICE_STATEMASK_ALL    uint32 = 0x0000000f
)

// ListDevies lists all audio devices, they need to be released after use
func ListDevices(mmde *wca.IMMDeviceEnumerator) []*Device {
	// Enumerate audio endpoints
	var dc *wca.IMMDeviceCollection
	var err error
	mmde.EnumAudioEndpoints(eCapture, DEVICE_STATEMASK_ALL, &dc)
	defer dc.Release()

	var count uint32
	dc.GetCount(&count)

	var devices []*Device

	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		dc.Item(i, &device)
		// Fetch device properties
		var ps *wca.IPropertyStore
		if err = device.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
			return nil
		}
		defer ps.Release()

		var pv wca.PROPVARIANT
		if err = ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
			return nil
		}
		devices = append(devices, &Device{name: pv, device: device})
	}
	return devices
}
