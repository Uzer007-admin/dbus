//go:build !windows && !darwin

package dbus

import (
	"net"
	"path/filepath"
	"testing"
)

func TestSessionDiscoveryRuntimeDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", directory)
	listener, err := net.Listen("unix", filepath.Join(directory, "bus"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	want := "unix:path=" + EscapeBusAddressValue(filepath.Join(directory, "bus"))
	if address := tryDiscoverDbusSessionBusAddress(); address != want {
		t.Fatalf("%q != %q", address, want)
	}
}
