package audio

import (
	"bytes"
	"sync"
)

type AudioStream struct {
	buffer *bytes.Buffer
	mu     *sync.RWMutex
}

// NewAudioStream creates a new AudioStream
func NewAudioStream() *AudioStream {
	return &AudioStream{
		buffer: &bytes.Buffer{},
	}
}

// Write appends data to the stream
func (s *AudioStream) Write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer.Write(data)
}

// Read reads data from the stream
func (s *AudioStream) Read() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buffer.Bytes()
}
