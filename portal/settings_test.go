package portal

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Uzer007-admin/dbus/v5"
)

type settingsObject struct {
	dbus.BusObject
	ctx    context.Context
	method string
	args   []any
	values map[string]map[string]dbus.Variant
}

func (object *settingsObject) CallWithContext(ctx context.Context, method string, _ dbus.Flags, args ...any) *dbus.Call {
	object.ctx, object.method, object.args = ctx, method, args
	return &dbus.Call{Err: ctx.Err(), Body: []any{object.values}}
}

func TestAppearancePreservesAvailabilityAndCancellation(t *testing.T) {
	object := &settingsObject{values: map[string]map[string]dbus.Variant{AppearanceNamespace: {
		"color-scheme": dbus.MakeVariant(uint32(1)), "contrast": dbus.MakeVariant(uint32(0)),
		"reduced-motion": dbus.MakeVariant(uint32(1)), "accent-color": dbus.MakeVariant(RGB{0.2, 0.4, 0.6}),
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	appearance, err := SettingsForObject(object).Appearance(ctx)
	if err != nil || !appearance.ColorScheme.Available || appearance.ColorScheme.Value != PreferDark || !appearance.HighContrast.Available || appearance.HighContrast.Value || !appearance.ReducedMotion.Value || !appearance.Accent.Available {
		t.Fatalf("%+v; %v", appearance, err)
	}
	if object.ctx != ctx || object.method != SettingsInterface+".ReadAll" || !reflect.DeepEqual(object.args, []any{[]string{AppearanceNamespace}}) {
		t.Fatal("method, namespace filter or context changed")
	}
	cancel()
	if _, err := SettingsForObject(object).Appearance(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	missing, err := ParseAppearance(nil)
	if err != nil || missing.ColorScheme.Available || missing.ReducedMotion.Available || missing.Accent.Available {
		t.Fatal("missing settings replaced by fabricated defaults")
	}
}

func TestAppearanceUnknownAndInvalidValues(t *testing.T) {
	appearance, err := ParseAppearance(map[string]dbus.Variant{
		"color-scheme": dbus.MakeVariant(uint32(99)), "contrast": dbus.MakeVariant(uint32(99)), "reduced-motion": dbus.MakeVariant(uint32(99)),
	})
	if err != nil || appearance.ColorScheme.Value != NoColorPreference || !appearance.ColorScheme.Available || appearance.HighContrast.Value || appearance.ReducedMotion.Value {
		t.Fatalf("unknown enums: %+v; %v", appearance, err)
	}
	for _, color := range []RGB{{-0.1, 0, 0}, {0, 1.1, 0}, {0, 0, math.NaN()}, {math.Inf(1), 0, 0}} {
		appearance, err := ParseAppearance(map[string]dbus.Variant{"accent-color": dbus.MakeVariant(color)})
		if err != nil || appearance.Accent.Available {
			t.Fatalf("invalid accent must be unset: %+v; %v", appearance, err)
		}
	}
	for key, value := range map[string]any{"color-scheme": true, "contrast": "high", "reduced-motion": int32(1), "accent-color": []float64{0, 0, 0}} {
		if _, err := ParseAppearance(map[string]dbus.Variant{key: dbus.MakeVariant(value)}); !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("%s wrong type accepted: %v", key, err)
		}
	}
}

func TestSettingChangedSignalShape(t *testing.T) {
	signal := &dbus.Signal{Name: SettingsInterface + ".SettingChanged", Path: SettingsPath, Body: []any{AppearanceNamespace, "color-scheme", dbus.MakeVariant(uint32(2))}}
	change, matched, err := ParseSettingChanged(signal)
	if err != nil || !matched || change.Key != "color-scheme" || change.Value.Value() != uint32(2) {
		t.Fatalf("%+v; %v", change, err)
	}
	signal.Body[2] = uint32(2)
	if _, matched, err := ParseSettingChanged(signal); !matched || !errors.Is(err, ErrInvalidSetting) {
		t.Fatal("malformed matching signal accepted")
	}
	signal.Path = "/another/object"
	if _, matched, err := ParseSettingChanged(signal); matched || err != nil {
		t.Fatal("unrelated object matched")
	}
}
