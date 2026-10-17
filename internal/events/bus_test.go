package events

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	defer cancel()
	b.Publish(Event{Type: "item.changed", ID: 3})
	select {
	case e := <-ch:
		if e.Type != "item.changed" || e.ID != 3 {
			t.Fatalf("%+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

func TestBusDropsSlow(t *testing.T) {
	b := NewBus()
	slow, cancel := b.Subscribe()
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Publish(Event{Type: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a slow subscriber")
	}
	if len(slow) != 64 {
		t.Fatalf("buffer %d", len(slow))
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	b := NewBus()
	ch, cancel := b.Subscribe()
	cancel()
	b.Publish(Event{Type: "x"})
	if _, ok := <-ch; ok {
		t.Fatal("event after cancel")
	}
}
