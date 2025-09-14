package orderedbuffer

import (
	"sync"
)

type OrderedBuffer[T any] struct {
	responses   map[int]T
	mu          *sync.RWMutex
	lastSentIdx int
	ch          chan T
}

func NewOrderedBuffer[T any](ch chan T) *OrderedBuffer[T] {
	var mu sync.RWMutex
	return &OrderedBuffer[T]{
		responses:   make(map[int]T),
		ch:          ch,
		lastSentIdx: -1,
		mu:          &mu,
	}
}

func (s *OrderedBuffer[T]) send() {
	s.mu.Lock()
	newIdx := s.lastSentIdx + 1
	if resp, ok := s.responses[newIdx]; ok {
		s.ch <- resp
		s.lastSentIdx = newIdx
		delete(s.responses, newIdx)
		s.mu.Unlock()
		s.send()
		return
	} else {
	}
	s.mu.Unlock()
}

func (s *OrderedBuffer[T]) Store(i int, r T) {
	s.mu.Lock()
	s.responses[i] = r
	s.mu.Unlock()
	s.send()
}
