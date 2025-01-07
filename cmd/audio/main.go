package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/soockee/bass-kata/audio"
	"github.com/soockee/bass-kata/capture"
	"github.com/soockee/bass-kata/render"
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
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(h))

	// Capture the start time
	startTime := time.Now()

	config := AppConfig{
		InputFile:     "data-test/piano2.wav",
		TempFile:      "data-test/output.wav",
		CaptureFile:   "data-test/captured_audio.wav",
		CaptureDevice: "Analogue 1 + 2 (Focusrite USB Audio)",
		OutputDevice:  "Speakers (Focusrite USB Audio)",
		// OutputDevice: "Headphones (2- High Definition Audio Device)",
		// CaptureDevice: "Microphone (Yeti Stereo Microphone)",
	}

	ctx, cancel := setupSignalHandling()
	defer cancel()

	if err := processWavFile(config.InputFile, config.TempFile); err != nil {
		slog.Error("Error processing WAV file", slog.Any("error", err))
		os.Exit(1)
	}

	runTasks(ctx, cancel, config)

	// Calculate and log the elapsed time
	elapsedTime := time.Since(startTime)
	slog.Debug("All tasks completed. Exiting.", slog.String("duration", elapsedTime.String()))

}

func runTasks(ctx context.Context, cancel context.CancelFunc, config AppConfig) {
	audiostream := audio.NewAudioStream()
	mux := audio.NewAudioMux(audiostream)

	// Add subscribers
	sub := mux.AddSubscriber()

	tasks := []struct {
		Name string
		Task func(context.Context) error
	}{
		{"Audio Capture File", func(ctx context.Context) error {
			return capture.CaptureFile(mux, config.TempFile, ctx)
		}},
		// {"Audio Capture Device", func(ctx context.Context) error {
		// 	return capture.CaptureDevice(mux, config.CaptureDevice, ctx)
		// }},
		// {"Audio Capture File", func(ctx context.Context) error {
		// 	return capture.CaptureDevice(mux, config.CaptureDevice, ctx)
		// }},
		{"Audio Rendering", func(ctx context.Context) error {
			return render.RenderFromStream(sub, config.OutputDevice, ctx)
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
				slog.Debug("Task completed successfully", slog.String("task", taskName))
			}
		}(task.Name, task.Task)
	}

	// Wait for all workers to finish
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

func processWavFile(inputFile, tempFile string) error {
	if err := audio.ConvertWav(inputFile, tempFile); err != nil {
		return err
	}
	slog.Debug("WAV file converted successfully", slog.String("file", tempFile))
	return nil
}
