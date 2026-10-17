package dbus

import (
	"errors"
	"sync"
	"sync/atomic"
)

var ErrInvalidSignalCapacity = errors.New("dbus: signal subscription capacity must be between 1 and 65536")

// SignalSubscription is an opt-in bounded signal stream independent of the
// connection's legacy SignalHandler. Delivery preserves incoming order and
// drops new signals when full, without blocking or spawning goroutines.
// After Overflow, a stateful consumer must re-read its authoritative state.
// Like Signal, this receives signals matched anywhere on a shared connection;
// validate sender, path, interface and body before applying them.
type SignalSubscription struct {
	registry *signalSubscriptions
	signals  chan *Signal
	overflow chan struct{}
	done     chan struct{}
	dropped  atomic.Uint64
}

// Signals returns the owned, read-only signal channel. Do not mutate delivered
// signals: other subscribers can receive the same object.
func (s *SignalSubscription) Signals() <-chan *Signal { return s.signals }

// Overflow coalesces notifications of dropped signals into one pending token.
// It remains open after shutdown; use Done to observe termination.
func (s *SignalSubscription) Overflow() <-chan struct{} { return s.overflow }
func (s *SignalSubscription) Dropped() uint64           { return s.dropped.Load() }
func (s *SignalSubscription) Done() <-chan struct{}     { return s.done }

// Close unregisters the subscription and closes Signals and Done. It is
// idempotent and does not close the supplied connection or remove bus rules.
func (s *SignalSubscription) Close() { s.registry.remove(s) }

type signalSubscriptions struct {
	mu      sync.RWMutex
	closed  bool
	streams map[*SignalSubscription]struct{}
}

// SubscribeSignals registers a bounded stream. It does not install bus match
// rules. This works alongside both built-in and caller-provided SignalHandlers;
// legacy Signal delivery semantics remain unchanged.
func (conn *Conn) SubscribeSignals(capacity int) (*SignalSubscription, error) {
	if capacity < 1 || capacity > 65536 {
		return nil, ErrInvalidSignalCapacity
	}
	registry := conn.signalSubscriptions.Load()
	if registry == nil {
		created := &signalSubscriptions{streams: make(map[*SignalSubscription]struct{})}
		if conn.signalSubscriptions.CompareAndSwap(nil, created) {
			registry = created
		} else {
			registry = conn.signalSubscriptions.Load()
		}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed || conn.signalSubscriptionsClosed.Load() || conn.ctx.Err() != nil {
		return nil, ErrClosed
	}
	subscription := &SignalSubscription{registry: registry, signals: make(chan *Signal, capacity), overflow: make(chan struct{}, 1), done: make(chan struct{})}
	registry.streams[subscription] = struct{}{}
	return subscription, nil
}

func (r *signalSubscriptions) deliver(signal *Signal) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for stream := range r.streams {
		select {
		case stream.signals <- signal:
		default:
			stream.dropped.Add(1)
			select {
			case stream.overflow <- struct{}{}:
			default:
			}
		}
	}
}

func (r *signalSubscriptions) remove(stream *SignalSubscription) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.streams[stream]; !exists {
		return
	}
	delete(r.streams, stream)
	close(stream.signals)
	close(stream.done)
}

func (r *signalSubscriptions) terminate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for stream := range r.streams {
		close(stream.signals)
		close(stream.done)
	}
	r.streams = nil
}
