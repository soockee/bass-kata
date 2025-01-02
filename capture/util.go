package capture

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/DylanMeeus/GoAudio/wave"
)

// type aliases for conversion functions
type (
	bytesToIntF func([]byte) int
)
type channelSelect int

var (
	// max value depending on the bit size
	maxValues = map[int]int{
		8:  math.MaxInt8,
		16: math.MaxInt16,
		32: math.MaxInt32,
		64: math.MaxInt64,
	}
	// figure out which 'to int' function to use..
	byteSizeToIntFunc = map[int]bytesToIntF{
		16: bits16ToInt,
		32: bits32ToInt,
	}
	MonoLeft  channelSelect = 1
	MonoRight channelSelect = 2
	Stereo    channelSelect = 3
)

// turn a 16-bit byte array into an int
func bits16ToInt(b []byte) int {
	if len(b) != 2 {
		panic("Expected size 4!")
	}
	var payload int16
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &payload)
	if err != nil {
		// TODO: make safe
		panic(err)
	}
	return int(payload) // easier to work with ints
}

// turn a 32-bit byte array into an int
func bits32ToInt(b []byte) int {
	if len(b) != 4 {
		panic("Expected size 4!")
	}
	var payload int32
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &payload)
	if err != nil {
		// TODO: make safe
		panic(err)
	}
	return int(payload) // easier to work with ints
}

// parseRawData takes raw audio data and converts it to a slice of audio frames
func parseRawData(wfmt wave.WaveFmt, rawdata []byte, channel channelSelect) []wave.Frame {
	bytesSampleSize := wfmt.BitsPerSample / 8
	frames := []wave.Frame{}

	if channel == Stereo {
		for i := 0; i < len(rawdata); i += bytesSampleSize {
			rawFrame := rawdata[i : i+bytesSampleSize]
			frameInt := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame)
			scaled := scaleFrame(frameInt, wfmt.BitsPerSample)
			frames = append(frames, scaled)
		}
		return frames
	}

	if channel == MonoLeft {
		for i := 0; i < len(rawdata); i += bytesSampleSize * 2 {
			rawFrame1 := rawdata[i : i+bytesSampleSize]
			// rawFrame2 := rawdata[i+bytesSampleSize : i+bytesSampleSize*2]
			frameInt1 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame1)
			// frameInt2 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame2)
			scaled1 := scaleFrame(frameInt1, wfmt.BitsPerSample)
			// scaled2 := scaleFrame(frameInt2, wfmt.BitsPerSample)

			frames = append(frames, scaled1, scaled1) // duplicate the frame
		}
	}
	if channel == MonoRight {
		for i := 0; i < len(rawdata); i += bytesSampleSize * 2 {
			// rawFrame1 := rawdata[i : i+bytesSampleSize]
			rawFrame2 := rawdata[i+bytesSampleSize : i+bytesSampleSize*2]
			// frameInt1 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame1)
			frameInt2 := byteSizeToIntFunc[wfmt.BitsPerSample](rawFrame2)
			// scaled1 := scaleFrame(frameInt1, wfmt.BitsPerSample)
			scaled2 := scaleFrame(frameInt2, wfmt.BitsPerSample)

			frames = append(frames, scaled2, scaled2) // duplicate the frame
		}
	}

	return frames
}

// scaleFrame scales an unscaled integer to a float64
func scaleFrame(unscaled, bits int) wave.Frame {
	maxV := maxValues[bits]
	return wave.Frame(float64(unscaled) / float64(maxV))
}
