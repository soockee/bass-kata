package audio

import (
	"sync"
)

// AudioMux wraps the AudioStream and manages subscribers
type AudioMux struct {
	Stream      *AudioStream
	subscribers []chan *Subscription
	mu          sync.Mutex
}

type Subscription struct {
	Stream   *AudioStream
	Position *Position
}

// NewAudioMux creates a new AudioMux for the given AudioStream
func NewAudioMux(stream *AudioStream) *AudioMux {
	return &AudioMux{
		Stream:      stream,
		subscribers: []chan *Subscription{},
	}
}

// AddSubscriber adds a new subscriber and returns its channel
func (m *AudioMux) AddSubscriber() <-chan *Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan *Subscription)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

// RemoveSubscriber removes a subscriber and closes its channel
func (m *AudioMux) RemoveSubscriber(subscriber <-chan *Subscription) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, ch := range m.subscribers {
		if ch == subscriber {
			close(ch)
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			break
		}
	}
}

// Broadcast writes data to all subscribers
func (m *AudioMux) Broadcast(pos Position) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub := &Subscription{Stream: m.Stream, Position: &pos}
	for _, ch := range m.subscribers {
		select {
		case ch <- sub:
		default: // Drop data if subscriber channel is full
		}
	}
}

// Write wraps the AudioStream's Write method and broadcasts data to subscribers
func (m *AudioMux) Write(data []byte) {
	l := m.Stream.Len()
	m.Stream.Write(data)
	m.Broadcast(Position{From: l, To: l + len(data)})
}

// Read exposes the Read functionality of the underlying AudioStream
func (m *AudioMux) Read(pos Position) []byte {
	return m.Stream.Read(pos)
}

// Close closes the stream and all subscriber channels
func (m *AudioMux) Close() {
	m.Stream.Close()
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = nil
}
