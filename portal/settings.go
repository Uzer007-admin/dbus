// Package portal provides typed access to standardized XDG desktop settings.
// It does not own the supplied connection, create goroutines, or apply UI policy.
package portal

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Uzer007-admin/dbus/v5"
)

const (
	SettingsDestination = "org.freedesktop.portal.Desktop"
	SettingsPath        = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	SettingsInterface   = "org.freedesktop.portal.Settings"
	AppearanceNamespace = "org.freedesktop.appearance"
)

var ErrInvalidSetting = errors.New("portal: invalid settings response")

// Settings queries an existing bus object. ReadAll is available in both
// versions 1 and 2 of the Settings interface; it avoids legacy Read's extra variant.
type Settings struct{ object dbus.BusObject }

func NewSettings(conn *dbus.Conn) *Settings {
	return SettingsForObject(conn.Object(SettingsDestination, SettingsPath))
}

// SettingsForObject permits explicit destinations and test/service objects.
func SettingsForObject(object dbus.BusObject) *Settings {
	return &Settings{object: object}
}

// ReadAll returns the raw namespace/key values. Context cancellation and bus
// errors are preserved; missing keys remain distinguishable from false/default.
func (s *Settings) ReadAll(ctx context.Context, namespaces ...string) (map[string]map[string]dbus.Variant, error) {
	var values map[string]map[string]dbus.Variant
	err := s.object.CallWithContext(ctx, SettingsInterface+".ReadAll", 0, namespaces).Store(&values)
	return values, err
}

func (s *Settings) Appearance(ctx context.Context) (Appearance, error) {
	values, err := s.ReadAll(ctx, AppearanceNamespace)
	if err != nil {
		return Appearance{}, err
	}
	return ParseAppearance(values[AppearanceNamespace])
}

// Preference preserves whether the portal supplied the key. Unknown enum values
// normalize to no preference, as specified by the portal interface.
type Preference[T any] struct {
	Value     T
	Available bool
}

type ColorScheme uint32

const (
	NoColorPreference ColorScheme = iota
	PreferDark
	PreferLight
)

type RGB struct{ Red, Green, Blue float64 }

// Appearance covers the four standardized appearance keys. Text scale, font,
// transparency permission and compositor blur are not standardized by this API.
type Appearance struct {
	Desktop       DesktopAppearance
	ColorScheme   Preference[ColorScheme]
	HighContrast  Preference[bool]
	ReducedMotion Preference[bool]
	Accent        Preference[RGB]
}

func ParseAppearance(values map[string]dbus.Variant) (Appearance, error) {
	var appearance Appearance
	for _, key := range []string{"color-scheme", "contrast", "reduced-motion", "accent-color"} {
		variant, present := values[key]
		if !present {
			continue
		}
		if err := applyAppearanceSetting(&appearance, key, variant); err != nil {
			return Appearance{}, err
		}
	}
	return appearance, nil
}

// ApplyAppearanceChange applies a standardized key to an immutable value
// snapshot without a map or a D-Bus read. recognized=false means an unrelated
// namespace/key. Invalid types return the original snapshot unchanged. Sender
// ownership, initial completeness and overflow recovery remain the watcher's
// responsibility; this helper alone does not authenticate a signal.
func ApplyAppearanceChange(appearance Appearance, change SettingChange) (Appearance, bool, error) {
	if change.Namespace != AppearanceNamespace || !appearanceKey(change.Key) {
		return appearance, false, nil
	}
	next := appearance
	if err := applyAppearanceSetting(&next, change.Key, change.Value); err != nil {
		return appearance, true, err
	}
	return next, true, nil
}

func appearanceKey(key string) bool {
	return key == "color-scheme" || key == "contrast" || key == "reduced-motion" || key == "accent-color"
}

func applyAppearanceSetting(appearance *Appearance, key string, variant dbus.Variant) error {
	if key == "accent-color" {
		if variant.Signature().String() != "(ddd)" {
			return fmt.Errorf("%w: accent-color must be (ddd)", ErrInvalidSetting)
		}
		var color RGB
		if err := variant.Store(&color); err != nil {
			return fmt.Errorf("%w: accent-color: %v", ErrInvalidSetting, err)
		}
		appearance.Accent = Preference[RGB]{}
		if validColor(color) {
			appearance.Accent = Preference[RGB]{color, true}
		}
		return nil
	}
	value, ok := variant.Value().(uint32)
	if !ok {
		return fmt.Errorf("%w: %s must be uint32", ErrInvalidSetting, key)
	}
	switch key {
	case "color-scheme":
		if value > 2 {
			value = 0
		}
		appearance.ColorScheme = Preference[ColorScheme]{ColorScheme(value), true}
	case "contrast":
		appearance.HighContrast = Preference[bool]{value == 1, true}
	case "reduced-motion":
		appearance.ReducedMotion = Preference[bool]{value == 1, true}
	}
	return nil
}

func validColor(color RGB) bool {
	for _, value := range [3]float64{color.Red, color.Green, color.Blue} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return false
		}
	}
	return true
}

type SettingChange struct {
	Namespace string
	Key       string
	Value     dbus.Variant
}

// ParseSettingChanged validates shape, interface/member and path. The caller
// must also validate signal.Sender against the portal's unique owner; a shared
// connection can deliver similarly named signals matched by other subscribers.
func ParseSettingChanged(signal *dbus.Signal) (change SettingChange, matched bool, err error) {
	if signal == nil || signal.Name != SettingsInterface+".SettingChanged" || signal.Path != SettingsPath {
		return SettingChange{}, false, nil
	}
	if len(signal.Body) != 3 {
		return SettingChange{}, true, fmt.Errorf("%w: SettingChanged needs three fields", ErrInvalidSetting)
	}
	namespace, namespaceOK := signal.Body[0].(string)
	key, keyOK := signal.Body[1].(string)
	value, valueOK := signal.Body[2].(dbus.Variant)
	if !namespaceOK || !keyOK || !valueOK {
		return SettingChange{}, true, fmt.Errorf("%w: SettingChanged must have signature ssv", ErrInvalidSetting)
	}
	return SettingChange{namespace, key, value}, true, nil
}
