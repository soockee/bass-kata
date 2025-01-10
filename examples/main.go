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
		// CaptureDevice: "Microphone (Yeti Stereo Microphone)",
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
