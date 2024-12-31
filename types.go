package main

import (
	"fmt"
	"strings"

	"github.com/moutend/go-wca/pkg/wca"
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

type Device struct {
	name   wca.PROPVARIANT
	device *wca.IMMDevice
}
