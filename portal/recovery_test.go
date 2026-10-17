package portal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Uzer007-admin/dbus/v5"
)

func TestWaitReadyBroadcastAndShutdown(t *testing.T) {
	w := &AppearanceWatcher{updates: make(chan AppearanceSnapshot, 1), done: make(chan struct{}), changed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var waiting sync.WaitGroup
	for range 16 {
		waiting.Go(func() {
			got, err := w.WaitReady(ctx)
			if err != nil || !got.Available || got.Appearance.ColorScheme.Value != PreferDark {
				t.Error(got, err)
			}
		})
	}
	w.publish(AppearanceSnapshot{Available: true, Appearance: Appearance{ColorScheme: Preference[ColorScheme]{PreferDark, true}}})
	waiting.Wait()
	if len(w.updates) != 1 {
		t.Fatal("WaitReady consumed the UI update")
	}
	w.publish(AppearanceSnapshot{Err: ErrWatcherClosed})
	close(w.done)
	if got, err := w.WaitReady(ctx); got.Available || !errors.Is(err, ErrWatcherClosed) {
		t.Fatal(got, err)
	}
}

func TestWaitReadyCancellation(t *testing.T) {
	w := &AppearanceWatcher{updates: make(chan AppearanceSnapshot, 1), done: make(chan struct{}), changed: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := w.WaitReady(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestRetryClassification(t *testing.T) {
	for _, err := range []error{dbus.Error{Name: "org.freedesktop.DBus.Error.NameHasNoOwner"}, &dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}} {
		if retryableAppearance(AppearanceSnapshot{Err: errors.Join(ErrSettingsUnavailable, err)}) {
			t.Fatal("missing service would be polled")
		}
	}
	if !retryableAppearance(AppearanceSnapshot{Owner: ":1.2", Err: errors.New("temporary read error")}) {
		t.Fatal("read would never recover")
	}
}
