package main

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/soockee/bass-kata/audio"
)

func main() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(h))

	inputFile := "data-test/burning_alive.wav"
	tmp_file := "data-test/output.wav"
	captureFile := "data-test/captured_audio.wav"
	captureDuration := 10 * time.Second // Example: Capture for 10 seconds
	captureDevicename := "Analogue 1 + 2 (Focusrite USB Audio)"

	if err := audio.ConvertWav(inputFile, tmp_file); err != nil {
		slog.Info("Error converting WAV file", slog.Any("Error", err))
	} else {
		slog.Info("WAV file converted successfully")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure resources are released when main exits

	var wg sync.WaitGroup

	// Run devices() in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := devices(ctx, cancel); err != nil {
			slog.Error("Error in devices", slog.String("error", err.Error()))
		}
	}()

	// Run render() in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := render(tmp_file, ctx, cancel); err != nil {
			slog.Error("Error in render", slog.String("error", err.Error()))
		}
	}()

	// Run capture() in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := capture(captureFile, ctx, cancel, captureDuration, captureDevicename); err != nil {
			slog.Error("Error in capture", slog.String("error", err.Error()))
		} else {
			slog.Info("Audio captured successfully", slog.String("File", captureFile))
		}
	}()

	// Wait for both goroutines to finish
	wg.Wait()
	slog.Info("All tasks completed. Exiting.")
}
