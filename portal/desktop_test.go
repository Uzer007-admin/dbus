package portal

import (
	"github.com/Uzer007-admin/dbus/v5"
	"testing"
)

func TestDesktopPaletteCompleteAndOpaque(t *testing.T) {
	values := map[string]map[string]dbus.Variant{
		DesktopNamespace + ".General":          {"ColorScheme": dbus.MakeVariant("BreezeDark")},
		DesktopNamespace + ".Colors:Window":    {"BackgroundNormal": dbus.MakeVariant("31,32,33"), "ForegroundNormal": dbus.MakeVariant("240,241,242"), "ForegroundInactive": dbus.MakeVariant("160,161,162")},
		DesktopNamespace + ".Colors:Button":    {"BackgroundNormal": dbus.MakeVariant("41,42,43"), "ForegroundNormal": dbus.MakeVariant("250,251,252")},
		DesktopNamespace + ".Colors:Selection": {"BackgroundNormal": dbus.MakeVariant("20,80,140"), "ForegroundNormal": dbus.MakeVariant("255,255,255")},
		DesktopNamespace + ".Colors:View":      {"ForegroundLink": dbus.MakeVariant("90,160,240")},
	}
	result := ParseDesktopAppearance(values)
	if !result.Colors.Available || result.Colors.Value.Window.R != 31 || result.Name.Value != "BreezeDark" {
		t.Fatal(result)
	}
	values[DesktopNamespace+".Colors:Window"]["BackgroundNormal"] = dbus.MakeVariant("31,32,33,0")
	if ParseDesktopAppearance(values).Colors.Available {
		t.Fatal("transparent palette accepted")
	}
	values[DesktopNamespace+".Colors:Window"]["BackgroundNormal"] = dbus.MakeVariant("999,32,33")
	if ParseDesktopAppearance(values).Colors.Available {
		t.Fatal("out of range palette accepted")
	}
	if ParseDesktopAppearance(nil).Colors.Available {
		t.Fatal("missing palette accepted")
	}
}
