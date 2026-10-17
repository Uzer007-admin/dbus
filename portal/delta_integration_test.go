//go:build linux

package portal

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Uzer007-admin/dbus/v5"
)

func TestAppearanceWatcherDeltaAvoidsReadAll(t *testing.T) {
	client, owner := settingsConnection(t), settingsConnection(t)
	name := fmt.Sprintf("org.godbus.DeltaSettings.p%d", os.Getpid())
	service := &mutableSettings{color: 1}
	ownSettings(t, owner, name, service)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: name, SignalCapacity: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	before := service.calls
	service.mu.Unlock()
	// Each complete, authenticated delta updates only its named preference.
	for i := 0; i < 20; i++ {
		value := uint32(1 + i%2)
		if err := owner.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant(value)); err != nil {
			t.Fatal(err)
		}
		waitAppearance(t, w, func(s AppearanceSnapshot) bool {
			return s.Available && s.Appearance.ColorScheme.Value == ColorScheme(value)
		})
	}
	if err := owner.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "reduced-motion", dbus.MakeVariant(uint32(1))); err != nil {
		t.Fatal(err)
	}
	snapshot := waitAppearance(t, w, func(s AppearanceSnapshot) bool {
		return s.Appearance.ReducedMotion.Available && s.Appearance.ReducedMotion.Value
	})
	if snapshot.Appearance.HighContrast.Available || snapshot.Appearance.Accent.Available {
		t.Fatal("delta invented absent settings")
	}
	service.mu.Lock()
	after := service.calls
	service.mu.Unlock()
	if after != before || w.DroppedSignals() != 0 {
		t.Fatalf("ordinary signals caused reads: before=%d after=%d dropped=%d", before, after, w.DroppedSignals())
	}
	// A wrong wire type from the trusted owner must recover from ReadAll,
	// not replace the last valid value with an unchecked variant.
	if err := owner.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant("bad")); err != nil {
		t.Fatal(err)
	}
	waitAppearance(t, w, func(s AppearanceSnapshot) bool {
		return s.Available && s.Appearance.ColorScheme.Value == PreferDark && !s.Appearance.ReducedMotion.Available
	})
	service.mu.Lock()
	after = service.calls
	service.mu.Unlock()
	if after != before+1 {
		t.Fatalf("malformed delta recovery calls=%d", after-before)
	}
}

type concurrentSnapshotSettings struct {
	mu      sync.Mutex
	conn    *dbus.Conn
	emitted bool
}

func (s *concurrentSnapshotSettings) ReadAll([]string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	color := uint32(2)
	if !s.emitted {
		s.emitted = true
		// A snapshot captured before a concurrent update may be returned after
		// SettingChanged. The watcher must not discard that newer signal just
		// because its sequence precedes the old ReadAll response.
		color = 1
		if err := s.conn.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(2))); err != nil {
			return nil, dbus.MakeFailedError(err)
		}
	}
	return map[string]map[string]dbus.Variant{AppearanceNamespace: {"color-scheme": dbus.MakeVariant(color)}}, nil
}

func TestAppearanceWatcherDeltaDuringRead(t *testing.T) {
	client, owner := settingsConnection(t), settingsConnection(t)
	name := fmt.Sprintf("org.godbus.ConcurrentSettings.p%d", os.Getpid())
	service := &concurrentSnapshotSettings{conn: owner}
	if err := owner.Export(service, SettingsPath, SettingsInterface); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: name})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available && s.Appearance.ColorScheme.Value == PreferLight })
}
