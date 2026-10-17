//go:build linux

package portal

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Uzer007-admin/dbus/v5"
)

type settingsService struct{}

func (settingsService) ReadAll(namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	if len(namespaces) != 1 || namespaces[0] != AppearanceNamespace {
		return nil, dbus.NewError("org.freedesktop.portal.Error.InvalidArgument", []any{"unexpected namespace filter"})
	}
	return map[string]map[string]dbus.Variant{AppearanceNamespace: {
		"color-scheme": dbus.MakeVariant(uint32(1)),
		"contrast":     dbus.MakeVariant(uint32(0)),
		"accent-color": dbus.MakeVariant(RGB{0.2, 0.4, 0.6}),
	}}, nil
}

func TestSettingsThroughDaemon(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires dbus-run-session")
	}
	service, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := service.Export(settingsService{}, SettingsPath, SettingsInterface); err != nil {
		t.Fatal(err)
	}
	// Use a unique connection name, so the test never takes over a desktop's
	// real portal service even when it runs outside an isolated test daemon.
	owner := service.Names()[0]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	appearance, err := SettingsForObject(client.Object(owner, SettingsPath)).Appearance(ctx)
	if err != nil || !appearance.Accent.Available || appearance.Accent.Value != (RGB{0.2, 0.4, 0.6}) ||
		appearance.ColorScheme.Value != PreferDark || appearance.ReducedMotion.Available {
		t.Fatalf("wire appearance: %+v; %v", appearance, err)
	}
	signals := make(chan *dbus.Signal, 4)
	client.Signal(signals)
	defer client.RemoveSignal(signals)
	rules := []dbus.MatchOption{dbus.WithMatchSender(owner), dbus.WithMatchObjectPath(SettingsPath),
		dbus.WithMatchInterface(SettingsInterface), dbus.WithMatchMember("SettingChanged")}
	if err := client.AddMatchSignalContext(ctx, rules...); err != nil {
		t.Fatal(err)
	}
	defer client.RemoveMatchSignal(rules...)
	if err := service.Emit(SettingsPath, SettingsInterface+".SettingChanged", AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(2))); err != nil {
		t.Fatal(err)
	}
	select {
	case signal := <-signals:
		change, matched, err := ParseSettingChanged(signal)
		if err != nil || !matched || signal.Sender != owner || change.Namespace != AppearanceNamespace ||
			change.Key != "color-scheme" || change.Value.Value() != uint32(2) {
			t.Fatalf("wire change: %+v; %+v; %v", signal, change, err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
