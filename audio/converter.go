package audio

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

func ConvertWav(inputFile, outputFile string) error {
	// Open the input file
	inFile, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inFile.Close()

	// Create the output file
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Read the input WAV file
	decoder := wav.NewDecoder(inFile)
	if !decoder.IsValidFile() {
		return fmt.Errorf("invalid WAV file")
	}

	if decoder.SampleRate != 48000 {
		slog.Debug("Converting sample rate to 48000 Hz", slog.Int("From", int(decoder.SampleRate)))
	}

	// Create the output WAV file
	encoder := wav.NewEncoder(outFile, 48000, int(decoder.BitDepth), int(decoder.NumChans), 1)

	// Read and write the samples
	buf := &audio.IntBuffer{Data: make([]int, 65536), Format: &audio.Format{SampleRate: 48000, NumChannels: 2}}

	for {
		numSamples, err := decoder.PCMBuffer(buf)
		if err != nil {
			if err == io.EOF {
				slog.Error("End of file reached")
				break
			}
			return fmt.Errorf("error reading PCM buffer from input file: %w", err)
		}

		// Check if buffer has valid data
		if numSamples == 0 {
			slog.Error("No samples read, stopping.")
			break
		}

		// Write only valid data
		buf.Data = buf.Data[:numSamples]
		if err := encoder.Write(buf); err != nil {
			return fmt.Errorf("error writing PCM buffer to output file: %w", err)
		}
	}

	// Close the encoder to write the headers
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error closing encoder: %w", err)
	}

	return nil
}
