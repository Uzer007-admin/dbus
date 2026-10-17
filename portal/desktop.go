package portal

import (
	"github.com/Uzer007-admin/dbus/v5"
	"image/color"
	"strconv"
	"strings"
)

const DesktopNamespace = "org.kde.kdeglobals"

// DesktopPalette содержит непрозрачные семантические цвета рабочего стола.
type DesktopPalette struct {
	Window, WindowText, ButtonFace, ButtonText   color.NRGBA
	Highlight, HighlightText, GrayText, Hotlight color.NRGBA
}

// DesktopAppearance описывает необязательное расширение портала KDE.
// Отсутствующие или некорректные цвета не подменяются вымышленными настройками.
type DesktopAppearance struct {
	Name, WidgetStyle, Font, IconTheme Preference[string]
	Colors                             Preference[DesktopPalette]
}

func ParseDesktopAppearance(values map[string]map[string]dbus.Variant) DesktopAppearance {
	text := func(group, key string) Preference[string] {
		value, ok := values[DesktopNamespace+"."+group][key].Value().(string)
		if !ok || value == "" || len(value) > 4096 {
			return Preference[string]{}
		}
		return Preference[string]{Value: value, Available: true}
	}
	result := DesktopAppearance{Name: text("General", "ColorScheme"), WidgetStyle: text("KDE", "widgetStyle"), Font: text("General", "font"), IconTheme: text("Icons", "Theme")}
	read := func(group, key string) (color.NRGBA, bool) {
		value, ok := values[DesktopNamespace+"."+group][key].Value().(string)
		if !ok {
			return color.NRGBA{}, false
		}
		parts := strings.Split(value, ",")
		if len(parts) != 3 && len(parts) != 4 {
			return color.NRGBA{}, false
		}
		c := [4]uint8{0, 0, 0, 255}
		for i, part := range parts {
			number, err := strconv.ParseUint(strings.TrimSpace(part), 10, 8)
			if err != nil {
				return color.NRGBA{}, false
			}
			c[i] = uint8(number)
		}
		if c[3] != 255 {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: c[0], G: c[1], B: c[2], A: 255}, true
	}
	var palette DesktopPalette
	roles := []struct {
		group, key string
		target     *color.NRGBA
	}{
		{"Colors:Window", "BackgroundNormal", &palette.Window}, {"Colors:Window", "ForegroundNormal", &palette.WindowText},
		{"Colors:Button", "BackgroundNormal", &palette.ButtonFace}, {"Colors:Button", "ForegroundNormal", &palette.ButtonText},
		{"Colors:Selection", "BackgroundNormal", &palette.Highlight}, {"Colors:Selection", "ForegroundNormal", &palette.HighlightText},
		{"Colors:Window", "ForegroundInactive", &palette.GrayText}, {"Colors:View", "ForegroundLink", &palette.Hotlight},
	}
	for _, role := range roles {
		value, ok := read(role.group, role.key)
		if !ok {
			return result
		}
		*role.target = value
	}
	result.Colors = Preference[DesktopPalette]{Value: palette, Available: true}
	return result
}
