package portal

import (
	"context"
	"testing"

	"github.com/Uzer007-admin/dbus/v5"
)

func TestWatcherOwnerValidationAndCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := WatchAppearance(ctx, nil, WatchOptions{}); err != context.Canceled {
		t.Fatal(err)
	}
	for _, name := range []string{"", ":1.5", "one", "org.bad'name", "org.5bad", "org..bad"} {
		if validDestination(name) {
			t.Fatalf("invalid destination %q", name)
		}
	}
	signal := &dbus.Signal{Sender: "org.freedesktop.DBus", Path: "/org/freedesktop/DBus", Name: "org.freedesktop.DBus.NameOwnerChanged", Body: []any{SettingsDestination, ":1.1", ":1.2"}}
	if owner, matched := changedOwner(signal, SettingsDestination); !matched || owner != ":1.2" {
		t.Fatal("valid owner change rejected")
	}
	signal.Sender = ":1.9"
	if _, matched := changedOwner(signal, SettingsDestination); matched {
		t.Fatal("other sender accepted as bus daemon")
	}
}
