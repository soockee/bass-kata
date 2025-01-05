package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime/pprof"
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

	f, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(f)

	defer pprof.StopCPUProfile()

	config := AppConfig{
		InputFile:     "data-test/burning_alive.wav",
		TempFile:      "data-test/output.wav",
		CaptureFile:   "data-test/captured_audio.wav",
		CaptureDevice: "Analogue 1 + 2 (Focusrite USB Audio)",
		// OutputDevice:  "Speakers (Focusrite USB Audio)",
		OutputDevice: "Headphones (2- High Definition Audio Device)",
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
	slog.Info("All tasks completed. Exiting.", slog.String("duration", elapsedTime.String()))

	memf, err := os.Create("mem.prof")
	if err != nil {
		log.Fatal(err)
	}
	pprof.WriteHeapProfile(memf)

}

func runTasks(ctx context.Context, cancel context.CancelFunc, config AppConfig) {
	audiostream := audio.NewAudioStream()

	tasks := []struct {
		Name string
		Task func(context.Context) error
	}{
		// {"Devices Monitoring", func(ctx context.Context) error {
		// 	return devices.Devices(ctx)
		// }},
		{"Audio Rendering", func(ctx context.Context) error {
			return render.Render(config.TempFile, config.OutputDevice, ctx)
		}},
		{"Audio Capture Stream", func(ctx context.Context) error {
			return capture.CaptureWithStream(audiostream, config.CaptureDevice, ctx)
		}},
		// {"Audio Capture File", func(ctx context.Context) error {
		// 	return capture.CaptureToFile(config.CaptureDevice, config.TempFile, ctx)
		// }},
		{"Audio Rendering", func(ctx context.Context) error {
			return render.RenderFromStream(audiostream, config.OutputDevice, ctx)
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

// func writeToFileFromStream(stream **capture.AudioStream, filename string) error {
// 	outputFile, err := os.Create(filename)
// 	if err != nil {
// 		return fmt.Errorf("failed to create output file: %w", err)
// 	}
// 	defer outputFile.Close()

// 	// Read and parse the `.wav` file
// 	err = wave.WriteFrames(samples, waveFmt, filename)
// 	if err != nil {
// 		return fmt.Errorf("failed to read file: %w", err)
// 	}
// 	slog.Debug("Audio capture successful", slog.String("file", filename))
// // Read and parse the `.wav` file
// 	err = wave.WriteFrames(samples, waveFmt, filename)
// 	if err != nil {
// 		return fmt.Errorf("failed to read file: %w", err)
// 	}
// }

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
	slog.Info("WAV file converted successfully", slog.String("file", tempFile))
	return nil
}
