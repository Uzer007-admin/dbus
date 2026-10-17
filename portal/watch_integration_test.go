//go:build linux

package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Uzer007-admin/dbus/v5"
)

type mutableSettings struct {
	mu       sync.Mutex
	color    uint32
	calls    int
	block    <-chan struct{}
	started  chan<- struct{}
	failures int
}

func (s *mutableSettings) ReadAll([]string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	s.calls++
	if s.failures > 0 {
		s.failures--
		s.mu.Unlock()
		return nil, dbus.NewError("org.godbus.Error.TemporarySettings", []interface{}{"try again"})
	}
	color, block, started := s.color, s.block, s.started
	s.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		<-block
	}
	return map[string]map[string]dbus.Variant{AppearanceNamespace: {"color-scheme": dbus.MakeVariant(color)}}, nil
}

func waitAppearance(t *testing.T, w *AppearanceWatcher, test func(AppearanceSnapshot) bool) AppearanceSnapshot {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case snapshot, open := <-w.Updates():
			if !open {
				t.Fatal("watcher stopped")
			}
			if test(snapshot) {
				return snapshot
			}
		case <-deadline.C:
			t.Fatalf("appearance did not converge: %+v", w.Snapshot())
		}
	}
}

func settingsConnection(t *testing.T) *dbus.Conn {
	t.Helper()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires dbus-run-session")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func ownSettings(t *testing.T, conn *dbus.Conn, name string, service *mutableSettings) {
	t.Helper()
	if err := conn.Export(service, SettingsPath, SettingsInterface); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
}

func TestAppearanceWatcherChangeSpoofRestartAndCleanup(t *testing.T) {
	client, first, rogue := settingsConnection(t), settingsConnection(t), settingsConnection(t)
	destination := fmt.Sprintf("org.godbus.VisualSettings.p%d", os.Getpid())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: destination, SignalCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return !s.Available && s.Err != nil })
	service := &mutableSettings{color: 1}
	ownSettings(t, first, destination, service)
	initial := waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available && s.Appearance.ColorScheme.Value == PreferDark })
	if initial.Owner != first.Names()[0] {
		t.Fatal("wrong authoritative owner")
	}
	// A separate subscriber's broad match makes rogue signals visible on this
	// shared connection. They must not alter state or request a settings read.
	extra := []dbus.MatchOption{dbus.WithMatchInterface(SettingsInterface)}
	if err := client.AddMatchSignal(extra...); err != nil {
		t.Fatal(err)
	}
	defer client.RemoveMatchSignal(extra...)
	service.mu.Lock()
	before := service.calls
	service.mu.Unlock()
	if err := rogue.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(2))); err != nil {
		t.Fatal(err)
	}
	// Barrier: the bus has dispatched the preceding emit before replying.
	var busID string
	if err := client.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetId", 0).Store(&busID); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	after := service.calls
	service.color = 2
	service.mu.Unlock()
	if after != before || w.Snapshot().Appearance.ColorScheme.Value != PreferDark {
		t.Fatal("rogue setting applied")
	}
	for i := 0; i < 100; i++ {
		if err := first.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(2))); err != nil {
			t.Fatal(err)
		}
	}
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available && s.Appearance.ColorScheme.Value == PreferLight })
	if _, err := first.ReleaseName(destination); err != nil {
		t.Fatal(err)
	}
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return !s.Available })
	second := settingsConnection(t)
	ownSettings(t, second, destination, &mutableSettings{color: 1})
	restarted := waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available && s.Owner == second.Names()[0] })
	if restarted.Appearance.ColorScheme.Value != PreferDark || restarted.Generation <= initial.Generation {
		t.Fatal("restart did not re-read new owner")
	}
	w.Close()
	if !client.Connected() {
		t.Fatal("watcher closed borrowed connection")
	}
	if err := client.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetId", 0).Store(&busID); err != nil {
		t.Fatal(err)
	}
}

func TestAppearanceWatcherCancelsReadFromOldOwner(t *testing.T) {
	client, first := settingsConnection(t), settingsConnection(t)
	destination := fmt.Sprintf("org.godbus.BlockedSettings.p%d", os.Getpid())
	block, started := make(chan struct{}), make(chan struct{}, 1)
	defer close(block)
	ownSettings(t, first, destination, &mutableSettings{color: 1, block: block, started: started})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := first.ReleaseName(destination); err != nil {
		t.Fatal(err)
	}
	second := settingsConnection(t)
	ownSettings(t, second, destination, &mutableSettings{color: 2})
	snapshot := waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available })
	if snapshot.Owner != second.Names()[0] || snapshot.Appearance.ColorScheme.Value != PreferLight {
		t.Fatalf("stale blocked result accepted: %+v", snapshot)
	}
	w.Close()
}

func TestAppearanceWatcherRecoversWithoutNewSignal(t *testing.T) {
	client, serviceConn := settingsConnection(t), settingsConnection(t)
	destination := fmt.Sprintf("org.godbus.RetrySettings.p%d", os.Getpid())
	service := &mutableSettings{color: 1, failures: 2}
	ownSettings(t, serviceConn, destination, service)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: destination, RetryInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	snapshot, err := w.WaitReady(ctx)
	if err != nil || !snapshot.Available || snapshot.Appearance.ColorScheme.Value != PreferDark {
		t.Fatalf("failed recovery: %+v %v", snapshot, err)
	}
	service.mu.Lock()
	calls := service.calls
	service.mu.Unlock()
	if calls != 3 {
		t.Fatalf("read calls: %d", calls)
	}
	client.Close()
	select {
	case <-w.Done():
	case <-ctx.Done():
		t.Fatal("watcher survived connection close")
	}
	if w.Snapshot().Available {
		t.Fatal("closed connection still advertised available settings")
	}
	if _, err := w.WaitReady(ctx); !errors.Is(err, ErrWatcherClosed) {
		t.Fatal(err)
	}
}
