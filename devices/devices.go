package devices

import (
	"fmt"

	"github.com/moutend/go-wca/pkg/wca"
)

type Device struct {
	Name   wca.PROPVARIANT
	Device *wca.IMMDevice
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

func FindDeviceByName(mmde *wca.IMMDeviceEnumerator, deviceName string) (*Device, error) {
	devices := ListDevices(mmde)
	// print all devices

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
