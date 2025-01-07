package audio

import (
	"errors"
	"sync"

	"github.com/DylanMeeus/GoAudio/wave"
)

// smallBufferSize is an initial allocation minimal capacity.
const smallBufferSize = 64
const maxInt = int(^uint(0) >> 1)

// ErrTooLarge is passed to panic if memory cannot be allocated to store data in a buffer.
var ErrTooLarge = errors.New("bytes.Buffer: too large")
var errNegativeRead = errors.New("bytes.Buffer: reader returned negative count from Read")

type AudioStream struct {
	Fmt         wave.WaveFmt
	buf         []byte
	subscribers []chan Position
	mu          sync.RWMutex
	once        sync.Once
	done        chan struct{}
	ready       chan struct{}
}

type Position struct {
	From int
	To   int
}

// NewAudioStream creates a new AudioStream
func NewAudioStream() *AudioStream {
	return &AudioStream{
		buf:         []byte{},
		ready:       make(chan struct{}),
		done:        make(chan struct{}),
		subscribers: []chan Position{},
	}
}

// Write appends the contents of p to the buffer, growing the buffer as
// needed. If buffer becomes too large, Write will panic with [ErrTooLarge].
// Write will notify all subscribers of the new data.
func (b *AudioStream) Write(p []byte) {
	m, ok := b.tryGrowByReslice(len(p))
	if !ok {
		m = b.grow(len(p))
	}
	copy(b.buf[m:], p)
	// Notify all subscribers
	for _, ch := range b.subscribers {
		select {
		case ch <- Position{From: m, To: m + len(p)}:
		default:
			// Drop data if subscriber channel is full
		}
	}
}

// SetFmt sets the audio format and signals readiness
func (s *AudioStream) SetFmt(fmt wave.WaveFmt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Fmt = fmt
	close(s.ready)
}

// Read reads data from the stream
func (s *AudioStream) Read(pos Position) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buf[pos.From:pos.To]
}

// Close closes the stream and signals that it is done
func (s *AudioStream) Close() {
	s.once.Do(func() {
		close(s.done)
		// Close all subscriber channels
		s.mu.Lock()
		for _, ch := range s.subscribers {
			close(ch)
		}
		s.subscribers = nil
		s.mu.Unlock()
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

// AddSubscriber adds a new subscriber to the stream
func (s *AudioStream) AddSubscriber() <-chan Position {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a new channel for the subscriber
	ch := make(chan Position)
	s.subscribers = append(s.subscribers, ch)
	return ch
}

// Len returns the number of bytes of the unread portion of the buffer;
// b.Len() == len(b.Bytes()).
func (b *AudioStream) Len() int { return len(b.buf) }

// tryGrowByReslice is an inlineable version of grow for the fast-case where the
// internal buffer only needs to be resliced.
// It returns the index where bytes should be written and whether it succeeded.
func (b *AudioStream) tryGrowByReslice(n int) (int, bool) {
	if l := len(b.buf); n <= cap(b.buf)-l {
		b.buf = b.buf[:l+n]
		return l, true
	}
	return 0, false
}

// grow grows the buffer to guarantee space for n more bytes.
// It returns the index where bytes should be written.
// If the buffer can't grow it will panic with ErrTooLarge.
func (b *AudioStream) grow(n int) int {
	m := b.Len()
	// If buffer is empty, reset to recover space.
	if m == 0 {
		b.Reset()
	}
	// Try to grow by means of a reslice.
	if i, ok := b.tryGrowByReslice(n); ok {
		return i
	}
	if b.buf == nil && n <= smallBufferSize {
		b.buf = make([]byte, n, smallBufferSize)
		return 0
	}
	c := cap(b.buf)
	if c > maxInt-c-n {
		panic(ErrTooLarge)
	} else {
		// Add b.off to account for b.buf[:b.off] being sliced off the front.
		b.buf = growSlice(b.buf, n)
	}
	b.buf = b.buf[:m+n]
	return m
}

// growSlice grows b by n, preserving the original content of b.
// If the allocation fails, it panics with ErrTooLarge.
func growSlice(b []byte, n int) []byte {
	defer func() {
		if recover() != nil {
			panic(ErrTooLarge)
		}
	}()
	// TODO(http://golang.org/issue/51462): We should rely on the append-make
	// pattern so that the compiler can call runtime.growslice. For example:
	//	return append(b, make([]byte, n)...)
	// This avoids unnecessary zero-ing of the first len(b) bytes of the
	// allocated slice, but this pattern causes b to escape onto the heap.
	//
	// Instead use the append-make pattern with a nil slice to ensure that
	// we allocate buffers rounded up to the closest size class.
	c := len(b) + n // ensure enough space for n elements
	if c < 2*cap(b) {
		// The growth rate has historically always been 2x. In the future,
		// we could rely purely on append to determine the growth rate.
		c = 2 * cap(b)
	}
	b2 := append([]byte(nil), make([]byte, c)...)
	copy(b2, b)
	return b2[:len(b)]
}

// Reset resets the AudioStream to be empty,
// but it retains the underlying storage for use by future writes.
func (b *AudioStream) Reset() {
	b.buf = b.buf[:0]
}

// Grow grows the buffer's capacity, if necessary, to guarantee space for
// another n bytes. After Grow(n), at least n bytes can be written to the
// buffer without another allocation.
// If n is negative, Grow will panic.
// If the buffer can't grow it will panic with [ErrTooLarge].
func (b *AudioStream) Grow(n int) {
	if n < 0 {
		panic("bytes.Buffer.Grow: negative count")
	}
	m := b.grow(n)
	b.buf = b.buf[:m]
}
