package dbus

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestBoundedSignalSubscriptionOverflowAndOrder(t *testing.T) {
	conn := &Conn{ctx: context.Background()}
	for _, capacity := range []int{0, -1, 65537} {
		if _, err := conn.SubscribeSignals(capacity); !errors.Is(err, ErrInvalidSignalCapacity) {
			t.Fatal(err)
		}
	}
	stream, err := conn.SubscribeSignals(2)
	if err != nil {
		t.Fatal(err)
	}
	registry := conn.signalSubscriptions.Load()
	for i := 0; i < 10000; i++ {
		registry.deliver(&Signal{Sequence: Sequence(i)})
	}
	if stream.Dropped() != 9998 || len(stream.signals) != 2 || len(stream.overflow) != 1 {
		t.Fatal("queue was not bounded")
	}
	if (<-stream.Signals()).Sequence != 0 || (<-stream.Signals()).Sequence != 1 {
		t.Fatal("signal order changed")
	}
	<-stream.Overflow()
	stream.Close()
	stream.Close()
	registry.deliver(&Signal{})
	select {
	case <-stream.Done():
	default:
		t.Fatal("close did not terminate subscription")
	}
	if _, open := <-stream.Signals(); open {
		t.Fatal("owned channel not closed")
	}
}

func TestBoundedSubscriptionConcurrentDeliveryAndTermination(t *testing.T) {
	conn := &Conn{ctx: context.Background()}
	stream, err := conn.SubscribeSignals(1)
	if err != nil {
		t.Fatal(err)
	}
	registry := conn.signalSubscriptions.Load()
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Go(func() {
			for i := 0; i < 1000; i++ {
				registry.deliver(&Signal{})
			}
		})
	}
	stream.Close()
	conn.signalSubscriptionsClosed.Store(true)
	registry.terminate()
	workers.Wait()
	if _, err := conn.SubscribeSignals(1); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func BenchmarkBoundedSignalDelivery(b *testing.B) {
	conn := &Conn{ctx: context.Background()}
	stream, err := conn.SubscribeSignals(1)
	if err != nil {
		b.Fatal(err)
	}
	defer stream.Close()
	registry, signal := conn.signalSubscriptions.Load(), &Signal{}
	b.ReportAllocs()
	for b.Loop() {
		registry.deliver(signal)
	}
}
