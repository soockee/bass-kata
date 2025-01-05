package audio

import (
	"github.com/DylanMeeus/GoAudio/wave"
	"github.com/moutend/go-wca/pkg/wca"
)

const (
	WAVE_FORMAT_EXTENSIBLE = 0xfffe
	WAVE_FORMAT_PCM        = 0x0001
)

// Compare compares two wavefmts and returns true if they are equal.
func CompareWaveFmt(a, b wave.WaveFmt) bool {
	for i := range a.Subchunk1ID {
		if a.Subchunk1ID[i] != b.Subchunk1ID[i] {
			return false
		}
	}
	return a.AudioFormat == b.AudioFormat &&
		a.NumChannels == b.NumChannels &&
		a.SampleRate == b.SampleRate &&
		a.BitsPerSample == b.BitsPerSample &&
		a.BlockAlign == b.BlockAlign &&
		a.ByteRate == b.ByteRate &&
		a.Subchunk1Size == b.Subchunk1Size
}

// Compares wave.WaveFmt and wca.WAVEFORMATEX
func CompareWaveFmtWfx(wf wave.WaveFmt, wfx *wca.WAVEFORMATEX) bool {
	return wf.AudioFormat == int(wfx.WFormatTag) &&
		wf.NumChannels == int(wfx.NChannels) &&
		wf.SampleRate == int(wfx.NSamplesPerSec) &&
		wf.BitsPerSample == int(wfx.WBitsPerSample)
}
