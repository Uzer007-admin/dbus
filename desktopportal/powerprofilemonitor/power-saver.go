package powerprofilemonitor

import (
	"github.com/Uzer007-admin/dbus/v5/desktopportal/internal/apis"
)

// GetPowerSaverEnabled reports whether “Power Saver” mode is enabled on the system.
func GetPowerSaverEnabled() (bool, error) {
	value, err := apis.GetProperty(interfaceName, "power-saver-enabled")
	if err != nil {
		return false, err
	}

	return value.(bool), nil
}
