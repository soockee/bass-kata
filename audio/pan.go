package audio

import (
	"math"

	"github.com/DylanMeeus/GoAudio/wave"
)

type Panposition struct {
	left, right float64
}

// calculateConstantPowerPosition finds the position of each speaker using a constant power function
func CalculateConstantPowerPosition(position float64) Panposition {
	// half a sinusoid cycle
	var halfpi float64 = math.Pi / 2
	r := math.Sqrt(2.0) / 2

	// scale position to fit in this range
	scaled := position * halfpi

	// each channel uses 1/4 of a cycle
	angle := scaled / 2
	pos := Panposition{}
	pos.left = r * (math.Cos(angle) - math.Sin(angle))
	pos.right = r * (math.Cos(angle) + math.Sin(angle))
	return pos
}

func CalculatePosition(position float64) Panposition {
	position *= 0.5
	return Panposition{
		left:  position - 0.5,
		right: position + 0.5,
	}
}

func ApplyPan(samples []wave.Frame, p Panposition) []wave.Frame {
	out := []wave.Frame{}
	for _, s := range samples {
		out = append(out, wave.Frame(float64(s)*p.left))
		out = append(out, wave.Frame(float64(s)*p.right))
	}
	return out
}
