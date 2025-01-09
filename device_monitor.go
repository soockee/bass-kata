package main

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// The resulting callback functions calls result to an invalid memory address error
func Devices(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	slog.Debug("Thread ID", slog.Int("tid", int(tid)), slog.String("function", "Devices"))

	// Initializes COM library for use by the calling thread
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
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
	// callback := wca.IMMNotificationClientCallback{
	// 	OnDefaultDeviceChanged: onDefaultDeviceChanged,
	// 	OnDeviceAdded:          onDeviceAdded,
	// 	OnDeviceRemoved:        onDeviceRemoved,
	// 	OnDeviceStateChanged:   onDeviceStateChanged,
	// 	OnPropertyValueChanged: onPropertyValueChanged,
	// }

	// Creates a notification client with the defined callbacks
	// mmnc := wca.NewIMMNotificationClient(callback)
	// mmnc := wca.NewIMMNotificationClient(wca.IMMNotificationClientCallback{})

	// // Registers the notification client with the audio device enumerator
	// if err := mmde.RegisterEndpointNotificationCallback(mmnc); err != nil {
	// 	return err
	// }

	// Main monitoring loop
	// for range ctx.Done() {
	// 	slog.Info("Context canceled, exiting devices monitoring.")
	// 	return nil
	// }
	return nil
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
