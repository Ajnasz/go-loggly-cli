package orderedbuffer

import (
	"testing"
	"time"
)

func TestOrderedBufferOrderedDelivery(t *testing.T) {
	ch := make(chan int, 3)
	buf := NewOrderedBuffer(ch)

	buf.Store(0, 10)
	buf.Store(1, 20)
	buf.Store(2, 30)

	got := []int{<-ch, <-ch, <-ch}
	want := []int{10, 20, 30}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %d at index %d, got %d", want[i], i, got[i])
		}
	}
}

func TestOrderedBufferOutOfOrder(t *testing.T) {
	ch := make(chan string, 3)
	buf := NewOrderedBuffer(ch)

	buf.Store(2, "c")
	buf.Store(0, "a")
	buf.Store(1, "b")

	got := []string{<-ch, <-ch, <-ch}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %s at index %d, got %s", want[i], i, got[i])
		}
	}
}

func TestOrderedBufferConcurrent(t *testing.T) {
	ch := make(chan int, 3)
	buf := NewOrderedBuffer(ch)

	go buf.Store(1, 100)
	go buf.Store(0, 50)
	go buf.Store(2, 150)

	time.Sleep(100 * time.Millisecond)
	got := []int{<-ch, <-ch, <-ch}
	want := []int{50, 100, 150}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %d at index %d, got %d", want[i], i, got[i])
		}
	}
}
