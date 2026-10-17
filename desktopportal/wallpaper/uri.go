package wallpaper

import (
	"github.com/Uzer007-admin/dbus/v5"
	"github.com/Uzer007-admin/dbus/v5/desktopportal/internal/apis"
)

// SetWallpaperURI sets wallpaper specified as a URI.
func SetWallpaperURI(parentWindow string, uri string, options *SetWallpaperOptions) error {
	data := dbusDataFromOptions(options)

	result, err := apis.Call(setWallpaperURICallName, parentWindow, uri, data)
	if err != nil {
		return err
	}

	return readStatusFromResponse(result.(dbus.ObjectPath))
}
