package portal

import (
	"errors"
	"math"
	"testing"

	"github.com/Uzer007-admin/dbus/v5"
)

func TestAppearanceDeltaMatchesFullParse(t *testing.T) {
	for _, key := range []string{"color-scheme", "contrast", "reduced-motion", "accent-color"} {
		for _, raw := range []any{uint32(0), uint32(1), uint32(2), uint32(99), "wrong", RGB{.1, .2, .3}, RGB{math.NaN(), .2, .3}, RGB{2, .2, .3}} {
			variant := dbus.MakeVariant(raw)
			base := Appearance{Accent: Preference[RGB]{RGB{.5, .6, .7}, true}}
			next, recognized, err := ApplyAppearanceChange(base, SettingChange{AppearanceNamespace, key, variant})
			full, fullErr := ParseAppearance(map[string]dbus.Variant{key: variant})
			if !recognized || (err == nil) != (fullErr == nil) {
				t.Fatalf("%s %v: %v %v", key, raw, err, fullErr)
			}
			if err != nil {
				if !errors.Is(err, ErrInvalidSetting) || next != base {
					t.Fatal("partial mutation", next, err)
				}
				continue
			}
			switch key {
			case "color-scheme":
				if next.ColorScheme != full.ColorScheme {
					t.Fatal(next, full)
				}
			case "contrast":
				if next.HighContrast != full.HighContrast {
					t.Fatal(next, full)
				}
			case "reduced-motion":
				if next.ReducedMotion != full.ReducedMotion {
					t.Fatal(next, full)
				}
			case "accent-color":
				if next.Accent != full.Accent {
					t.Fatal(next, full)
				}
			}
		}
	}
	for _, change := range []SettingChange{{"org.example", "color-scheme", dbus.MakeVariant(uint32(1))}, {AppearanceNamespace, "unknown", dbus.MakeVariant(uint32(1))}} {
		if _, recognized, err := ApplyAppearanceChange(Appearance{}, change); recognized || err != nil {
			t.Fatal(change, err)
		}
	}
}

var deltaBenchmarkSink Appearance

func BenchmarkAppearanceDelta(b *testing.B) {
	change := SettingChange{AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(1))}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		deltaBenchmarkSink, _, _ = ApplyAppearanceChange(deltaBenchmarkSink, change)
	}
}
