package wallpaper

import (
	"github.com/Uzer007-admin/dbus/v5"
	"github.com/Uzer007-admin/dbus/v5/desktopportal/internal/apis"
)

// SetWallpaperFile sets wallpaper specified as a local file.
func SetWallpaperFile(parentWindow string, fd uintptr, options *SetWallpaperOptions) error {
	data := dbusDataFromOptions(options)

	result, err := apis.Call(setWallpaperFileCallName, parentWindow, dbus.UnixFD(fd), data)
	if err != nil {
		return err
	}

	return readStatusFromResponse(result.(dbus.ObjectPath))
}
