package orderedbuffer

import (
	"sync"
)

type OrderedBuffer[T any] struct {
	responses   map[int]T
	mu          sync.Mutex
	lastSentIdx int
	ch          chan T
}

func NewOrderedBuffer[T any](ch chan T) *OrderedBuffer[T] {
	return &OrderedBuffer[T]{
		responses:   make(map[int]T),
		ch:          ch,
		lastSentIdx: -1,
	}
}

func (s *OrderedBuffer[T]) send() {
	for {
		s.mu.Lock()
		newIdx := s.lastSentIdx + 1
		resp, ok := s.responses[newIdx]
		if !ok {
			s.mu.Unlock()
			return
		}
		s.lastSentIdx = newIdx
		delete(s.responses, newIdx)
		s.mu.Unlock()

		s.ch <- resp
	}
}

func (s *OrderedBuffer[T]) Store(i int, r T) {
	s.mu.Lock()
	s.responses[i] = r
	s.mu.Unlock()

	s.send()
}
