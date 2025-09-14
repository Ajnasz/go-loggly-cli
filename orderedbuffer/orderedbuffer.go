package orderedbuffer

import (
	"fmt"
	"os"
	"sync"
)

type OrderedBuffer[T any] struct {
	responses   map[int]T
	mu          *sync.RWMutex
	lastSentIdx int
	ch          chan T
}

func NewOrderedBuffer[T any](ch chan T) *OrderedBuffer[T] {
	fmt.Fprintln(os.Stderr, "Creating new ResponsesStore")
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
	fmt.Fprintln(os.Stderr, "Checking for page", newIdx)
	if resp, ok := s.responses[newIdx]; ok {
		fmt.Fprintln(os.Stderr, "Sending page", newIdx)
		s.ch <- resp
		s.lastSentIdx = newIdx
		delete(s.responses, newIdx)
		s.mu.Unlock()
		s.send()
		return
	} else {
		fmt.Fprintln(os.Stderr, "Page", newIdx, "not ready yet")
	}
	s.mu.Unlock()
}

func (s *OrderedBuffer[T]) Store(i int, r T) {
	fmt.Fprintf(os.Stderr, "Storing page %d\n", i)
	s.mu.Lock()
	s.responses[i] = r
	s.mu.Unlock()
	s.send()
}
