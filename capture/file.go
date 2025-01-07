package capture

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DylanMeeus/GoAudio/wave"

	"github.com/soockee/bass-kata/audio"
)

func CaptureFile(mux *audio.AudioMux, filename string, ctx context.Context) error {
	// Ensure input file is provided
	if filename == "" {
		return fmt.Errorf("specify WAVE audio file (*.wav)")
	}

	// Read and parse the `.wav` file
	d, err := wave.ReadWaveFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	waveFmt := wave.NewWaveFmt(
		d.AudioFormat,
		d.NumChannels,
		d.SampleRate,
		d.BitsPerSample,
		nil,
	)

	mux.Stream.SetFmt(waveFmt)

	slog.Debug("Wave format", slog.Any("waveFmt", waveFmt))

	dd := [][]byte{}
	for i := 0; i < len(d.RawData); i += 4 {
		dd = append(dd, audio.GenerateSine(mux))
	}

	for _, d := range dd {
		mux.Stream.Write(d)
	}

	mux.Stream.Close()

	return nil
}
