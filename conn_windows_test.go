package dbus

import "testing"

func TestWindowsSystemBusAddress(t *testing.T) {
	for _, value := range []string{"", "tcp:host=localhost,port=12345"} {
		t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", value)
		address, err := getSystemBusPlatformAddress()
		want := value
		if want == "" {
			want = defaultSystemBusAddress
		}
		if err != nil || address != want {
			t.Fatalf("got %q %v, want %q", address, err, want)
		}
	}
}
