# go-record

[![Go Reference](https://pkg.go.dev/badge/github.com/soockee/go-record.svg)](https://pkg.go.dev/github.com/yourusername/go-record)
[![Build Status](https://github.com/soockee/go-record/actions/workflows/test.yml/badge.svg)](https://github.com/soockee/go-record/actions?query=workflow%3Atest)

A library to capture and render audio using WASAPI.

- [go-record](#go-record)
  - [Overview](#overview)
  - [Platforms](#platforms)
  - [Prerequisite](#prerequisite)
    - [Windows](#windows)
  - [Usage](#usage)
    - [Capturing Audio](#capturing-audio)
    - [Rendering Audio](#rendering-audio)
    - [Listing Devices](#listing-devices)
    - [Finding a Device by Name](#finding-a-device-by-name)
  - [Functions](#functions)
    - [Capture](#capture)
    - [Render](#render)
    - [ListDevices](#listdevices)
    - [FindDeviceByName](#finddevicebyname)

## Overview

This project provides functionalities to capture and render audio using WASAPI. It includes functions to list audio devices, capture audio from a specified device, and render audio to a specified device.

## Platforms

- Windows

## Prerequisite

### Windows

No additional prerequisites are required.

## Usage

### Capturing Audio

The following is an example of capturing audio from a specified device:

```go
package main

import (
    "context"
    "log"

    "github.com/soockee/go-record"
)

func main() {
    stream := record.NewAudioStream()
    ctx := context.Background()
    err := record.Capture(stream, "Microphone (Realtek High Definition Audio)", ctx)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Rendering Audio

The following is an example of rendering audio to a specified device:

```go
package main

import (
    "context"
    "log"

    "github.com/soockee/go-record"
)

func main() {
    stream := record.NewAudioStream()
    ctx := context.Background()
    err := record.Render(stream, "Speakers (Realtek High Definition Audio)", ctx)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Listing Devices

The following is an example of listing all active render devices:

```go
package main

import (
    "fmt"

    "github.com/moutend/go-wca/pkg/wca"
    "github.com/soockee/go-record"
)

func main() {
    mmde := wca.NewIMMDeviceEnumerator()
    devices := record.ListDevices(mmde, record.ERender, record.DEVICE_STATE_ACTIVE)
    for _, device := range devices {
        fmt.Println(device.Name.String())
    }
}
```

### Finding a Device by Name

The following is an example of finding a device by its name:

```go
package main

import (
    "fmt"
    "log"

    "github.com/moutend/go-wca/pkg/wca"
    "github.com/soockee/go-record"
)

func main() {
    mmde := wca.NewIMMDeviceEnumerator()
    device, err := record.FindDeviceByName(mmde, "Speakers (Realtek High Definition Audio)", record.ERender, record.DEVICE_STATE_ACTIVE)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(device.Name.String())
}
```

## Functions

### Capture

The `Capture` function captures audio from a specified device and exposes it as a stream.

```go
func Capture(stream *AudioStream, deviceName string, ctx context.Context) error
```

- `stream`: The `AudioStream` object to which the captured audio will be written.
- `deviceName`: The name of the audio device to capture from.
- `ctx`: The context to control the capture process.

### Render

The `Render` function renders audio to a specified device from a provided audio stream.

```go
func Render(stream *AudioStream, deviceName string, ctx context.Context) error
```

- `stream`: The `AudioStream` object from which the audio will be read.
- `deviceName`: The name of the audio device to render to.
- `ctx`: The context to control the rendering process.

### ListDevices

The `ListDevices` function lists all audio devices of a specified type and state.

```go
func ListDevices(mmde *wca.IMMDeviceEnumerator, deviceType DeviceType, deviceState DeviceState) []*Device
```

- `mmde`: The device enumerator object.
- `deviceType`: The type of devices to list (e.g., `ERender`, `ECapture`).
- `deviceState`: The state of devices to list (e.g., `DEVICE_STATE_ACTIVE`).

### FindDeviceByName

The `FindDeviceByName` function finds an audio device by its name.

```go
func FindDeviceByName(mmde *wca.IMMDeviceEnumerator, deviceName string, deviceType DeviceType, deviceState DeviceState) (*Device, error)
```

- `mmde`: The device enumerator object.
- `deviceName`: The name of the device to find.
- `deviceType`: The type of device to find (e.g., `ERender`, `ECapture`).
- `deviceState`: The state of the device to find (e.g., `DEVICE_STATE_ACTIVE`).
