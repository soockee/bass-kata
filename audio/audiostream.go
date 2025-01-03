package audio

import (
	"bytes"
	"sync"

	"github.com/DylanMeeus/GoAudio/wave"
)

type AudioStream struct {
	Fmt    wave.WaveFmt
	buffer *bytes.Buffer
	mu     sync.RWMutex
	once   sync.Once
	done   chan struct{}
	ready  chan struct{}
}

// NewAudioStream creates a new AudioStream
func NewAudioStream() *AudioStream {
	return &AudioStream{
		buffer: &bytes.Buffer{},
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Write appends data to the stream
func (s *AudioStream) Write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer.Write(data)
}

// SetFmt sets the audio format and signals readiness
func (s *AudioStream) SetFmt(fmt wave.WaveFmt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Fmt = fmt
	close(s.ready)
}

// Read reads data from the stream
func (s *AudioStream) Read() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buffer.Bytes()
}

// Close closes the stream and signals that it is done
func (s *AudioStream) Close() {
	s.once.Do(func() {
		close(s.done)
	})
}

// Done returns a channel that is closed when the stream is closed
func (s *AudioStream) Done() <-chan struct{} {
	return s.done
}

// Ready returns a channel that is closed when the Fmt is set
func (s *AudioStream) Ready() <-chan struct{} {
	return s.ready
}
