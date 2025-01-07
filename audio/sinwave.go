package audio

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/faiface/beep"
)

type SineWave struct {
	sampleFactor float64 // Just for ease of use so that we don't have to calculate every sample
	phase        float64
}

func (g *SineWave) Stream(samples [][2]float64) (n int, ok bool) {
	for i := range samples { // increment = ((2 * PI) / SampleRate) * freq
		v := math.Sin(g.phase * 2.0 * math.Pi) // period of the wave is thus defined as: 2 * PI.
		samples[i][0] = v
		samples[i][1] = v
		_, g.phase = math.Modf(g.phase + g.sampleFactor)
	}

	return len(samples), true
}

func (*SineWave) Err() error {
	return nil
}

func SineTone(sr beep.SampleRate, freq float64) (beep.Streamer, error) {
	dt := freq / float64(sr)

	if dt >= 1.0/2.0 {
		return nil, errors.New("samplerate must be at least 2 times grater then frequency")
	}

	return &SineWave{dt, 0.1}, nil
}

func GenerateSine(mux *AudioMux) []byte {
	const (
		start      float64 = 1.0    // Initial amplitude
		end        float64 = 1.0e-4 // Final amplitude after decay
		SampleRate int     = 44100  // Samples per second
		Duration   int     = 5      // Duration of the sine wave in seconds
		Frequency  float64 = 440.0  // Frequency of the sine wave in Hz
	)
	const tau = 2 * math.Pi // Tau is 2π

	// Total number of samples
	nsamps := Duration * SampleRate

	// Angle increment per sample
	angle := tau / float64(SampleRate)

	// Decay factor for amplitude
	decayfac := math.Pow(end/start, 1.0/float64(nsamps))

	// Buffer to accumulate samples (1024 bytes = 256 samples for 32-bit floats)
	buffer := make([]byte, 1024)
	bufIndex := 0

	// Current amplitude
	amplitude := start

	for i := 0; i < nsamps; i++ {
		// Generate sine wave sample
		sample := math.Sin(angle * Frequency * float64(i))
		sample *= amplitude

		// Apply amplitude decay
		amplitude *= decayfac

		// Convert the sample to float32 and then to byte array
		var sampleBytes [4]byte
		binary.LittleEndian.PutUint32(sampleBytes[:], math.Float32bits(float32(sample)))

		// Add the sample bytes to the buffer
		copy(buffer[bufIndex:bufIndex+4], sampleBytes[:])
		bufIndex += 4

		// If the buffer is full, write it to the mux stream
		if bufIndex >= len(buffer) {
			// mux.Write(buffer)

			bufIndex = 0 // Reset buffer index after writing
		}
	}

	// Write any remaining samples in the buffer
	if bufIndex > 0 {
		// mux.Write(buffer[:bufIndex])
	}
	return buffer
}
