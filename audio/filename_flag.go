package audio

import (
	"fmt"
	"strings"
)

type FilenameFlag struct {
	Value string
}

func (f *FilenameFlag) Set(value string) (err error) {
	if !strings.HasSuffix(value, ".wav") {
		err = fmt.Errorf("specify WAVE audio file (*.wav)")
		return
	}
	f.Value = value
	return
}

func (f *FilenameFlag) String() string {
	return f.Value
}
