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

type desktopSettingsService struct {
	mu         sync.Mutex
	name       string
	background string
	calls      int
}

func (s *desktopSettingsService) ReadAll(namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(namespaces) != 2 || namespaces[0] != AppearanceNamespace || namespaces[1] != DesktopNamespace+".*" {
		return nil, dbus.NewError("org.godbus.Error.Namespace", nil)
	}
	s.calls++
	return map[string]map[string]dbus.Variant{
		AppearanceNamespace:                    {"color-scheme": dbus.MakeVariant(uint32(1))},
		DesktopNamespace + ".General":          {"ColorScheme": dbus.MakeVariant(s.name), "font": dbus.MakeVariant("Noto Sans,10")},
		DesktopNamespace + ".KDE":              {"widgetStyle": dbus.MakeVariant("Union")},
		DesktopNamespace + ".Colors:Window":    {"BackgroundNormal": dbus.MakeVariant(s.background), "ForegroundNormal": dbus.MakeVariant("240,241,242"), "ForegroundInactive": dbus.MakeVariant("160,161,162")},
		DesktopNamespace + ".Colors:Button":    {"BackgroundNormal": dbus.MakeVariant("41,42,43"), "ForegroundNormal": dbus.MakeVariant("250,251,252")},
		DesktopNamespace + ".Colors:Selection": {"BackgroundNormal": dbus.MakeVariant("20,80,140"), "ForegroundNormal": dbus.MakeVariant("255,255,255")},
		DesktopNamespace + ".Colors:View":      {"ForegroundLink": dbus.MakeVariant("90,160,240")},
	}, nil
}

func TestDesktopPaletteWatcherRereadsWholePalette(t *testing.T) {
	client, server, rogue := settingsConnection(t), settingsConnection(t), settingsConnection(t)
	destination := fmt.Sprintf("org.godbus.DesktopPalette.p%d", os.Getpid())
	service := &desktopSettingsService{name: "BreezeDark", background: "31,32,33"}
	if err := server.Export(service, SettingsPath, SettingsInterface); err != nil {
		t.Fatal(err)
	}
	if _, err := server.RequestName(destination, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, err := WatchAppearance(ctx, client, WatchOptions{Destination: destination, IncludeDesktopSettings: true})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Available && s.Appearance.Desktop.Colors.Available })
	service.mu.Lock()
	service.name = "Custom"
	service.background = "60,61,62"
	service.mu.Unlock()
	// KDE сообщает о смене схемы одним событием; цвет требуется перечитать.
	if err := server.Emit(SettingsPath, SettingsInterface+".SettingChanged", DesktopNamespace+".General", "ColorScheme", dbus.MakeVariant("Custom")); err != nil {
		t.Fatal(err)
	}
	snapshot := waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Appearance.Desktop.Name.Value == "Custom" })
	if snapshot.Appearance.Desktop.Colors.Value.Window.R != 60 || snapshot.Appearance.Desktop.WidgetStyle.Value != "Union" {
		t.Fatal(snapshot)
	}
	// Посторонний отправитель не должен менять даже метаданные стиля.
	if err := rogue.Emit(SettingsPath, SettingsInterface+".SettingChanged", DesktopNamespace+".KDE", "widgetStyle", dbus.MakeVariant("Other")); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.background = "invalid"
	service.name = "Broken"
	service.mu.Unlock()
	if err := server.Emit(SettingsPath, SettingsInterface+".SettingChanged", DesktopNamespace+".General", "ColorScheme", dbus.MakeVariant("Broken")); err != nil {
		t.Fatal(err)
	}
	snapshot = waitAppearance(t, w, func(s AppearanceSnapshot) bool { return s.Appearance.Desktop.Name.Value == "Broken" })
	if snapshot.Appearance.Desktop.Colors.Available || !snapshot.Appearance.ColorScheme.Available || snapshot.Appearance.Desktop.WidgetStyle.Value != "Union" {
		t.Fatal(snapshot)
	}
	if _, err := server.ReleaseName(destination); err != nil {
		t.Fatal(err)
	}
	waitAppearance(t, w, func(s AppearanceSnapshot) bool { return !s.Available })
}
