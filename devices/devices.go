package devices

import (
	"fmt"

	"github.com/moutend/go-wca/pkg/wca"
)

type Device struct {
	Name   wca.PROPVARIANT
	Device *wca.IMMDevice
}

type DeviceType uint32

type DeviceState uint32

const (
	// EDataFlow enum
	ERender              DeviceType = 0
	ECapture             DeviceType = 1
	EAll                 DeviceType = 2
	EDataFlow_enum_count DeviceType = 3
	// DeviceState enum
	DEVICE_STATE_ACTIVE     DeviceState = 0x00000001
	DEVICE_STATE_DISABLED   DeviceState = 0x00000002
	DEVICE_STATE_NOTPRESENT DeviceState = 0x00000004
	DEVICE_STATE_UNPLUGGED  DeviceState = 0x00000008
	DEVICE_STATEMASK_ALL    DeviceState = 0x0000000f
)

// ListDevies lists all audio devices, they need to be released after use
func ListDevices(mmde *wca.IMMDeviceEnumerator, deviceType DeviceType, deviceState DeviceState) []*Device {
	// Enumerate audio endpoints
	var dc *wca.IMMDeviceCollection
	var err error
	mmde.EnumAudioEndpoints(uint32(deviceType), uint32(deviceState), &dc)
	defer dc.Release()

	var count uint32
	dc.GetCount(&count)

	var devices []*Device
	var device *wca.IMMDevice

	for i := uint32(0); i < count; i++ {
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
		devices = append(devices, &Device{Name: pv, Device: device})
	}
	return devices
}

func FindDeviceByName(mmde *wca.IMMDeviceEnumerator, deviceName string, deviceType DeviceType, deviceState DeviceState) (*Device, error) {
	devices := ListDevices(mmde, deviceType, deviceState)
	// print all devices
	// for _, device := range devices {
	// 	fmt.Println(device.Name.String())
	// }

	var d *Device
	for _, device := range devices {
		if device.Name.String() == deviceName {
			d = device
		} else {
			device.Device.Release()
		}
	}
	if d != nil {
		return d, nil
	}
	return nil, fmt.Errorf("device not found: %s", deviceName)
}
