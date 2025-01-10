# go-record

[![Go Reference](https://pkg.go.dev/badge/github.com/soockee/go-record.svg)](https://pkg.go.dev/github.com/soockee/go-record)
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
  - [Examples](#examples)
    - [Example Main](#example-main)

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

## Examples

### Example Main

The following is an example of a main function that captures and renders audio:

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "sync"

    "github.com/soockee/go-record"
)

type AppConfig struct {
    InputFile     string
    TempFile      string
    CaptureFile   string
    CaptureDevice string
    OutputDevice  string
}

func main() {
    h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    slog.SetDefault(slog.New(h))

    config := AppConfig{
        InputFile:     "data-test/burning_alive.wav",
        TempFile:      "data-test/output.wav",
        CaptureFile:   "data-test/captured_wav",
        CaptureDevice: "Analogue 1 + 2 (Focusrite USB Audio)",
        OutputDevice:  "Speakers (Focusrite USB Audio)",
    }

    ctx, cancel := setupSignalHandling()
    defer cancel()
    runTasks(ctx, cancel, config)
}

func runTasks(ctx context.Context, cancel context.CancelFunc, config AppConfig) {
    audiostream := record.NewAudioStream()

    tasks := []struct {
        Name string
        Task func(context.Context) error
    }{
        {"Audio Capture Stream", func(ctx context.Context) error {
            return record.Capture(audiostream, config.CaptureDevice, ctx)
        }},
        {"Audio Rendering", func(ctx context.Context) error {
            return record.Render(audiostream, config.OutputDevice, ctx)
        }},
    }

    var wg sync.WaitGroup

    for _, task := range tasks {
        wg.Add(1)
        go func(taskName string, taskFunc func(context.Context) error) {
            defer wg.Done()
            if err := taskFunc(ctx); err != nil {
                slog.Error("Task Event", slog.String("task", taskName), slog.Any("msg", err))
                cancel()
            } else {
                slog.Info("Task completed successfully", slog.String("task", taskName))
            }
        }(task.Name, task.Task)
    }

    wg.Wait()
}

func setupSignalHandling() (context.Context, context.CancelFunc) {
    ctx, cancel := context.WithCancel(context.Background())

    // Catch OS signals like Ctrl+C
    signalChan := make(chan os.Signal, 1)
    signal.Notify(signalChan, os.Interrupt)

    go func() {
        <-signalChan
        slog.Info("SIGINT received, shutting down...")
        cancel()
    }()

    return ctx, cancel
}
```
