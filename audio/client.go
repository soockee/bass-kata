package audio

import (
	"context"
	"fmt"

	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/moutend/go-wca/pkg/wca"
	"github.com/soockee/bass-kata/devices"
)

type AudioClientOpt struct {
	DeviceName string
	WaveFmt    wave.WaveFmt
	Wfx        *wca.WAVEFORMATEX
	Ctx        context.Context
	Mode       uint32
}

func SetupAudioClient(deviceName string) (*wca.IAudioClient3, error) {
	// Get default capture audio endpoint
	var mmde *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &mmde); err != nil {
		return nil, fmt.Errorf("failed to create device enumerator: %w", err)
	}
	defer mmde.Release()

	// Find the desired device by name
	device, err := devices.FindDeviceByName(mmde, deviceName, devices.EAll, devices.DEVICE_STATE_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("failed to find device by name: %w", err)
	}
	defer device.Device.Release()

	var ac *wca.IAudioClient3
	if err := device.Device.Activate(wca.IID_IAudioClient3, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return nil, fmt.Errorf("failed to activate audio client: %w", err)
	}

	return ac, nil
}