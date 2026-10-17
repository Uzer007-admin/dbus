//go:build linux

package dbus

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
)

type fdContainerService struct{}

func (fdContainerService) ReadArray(descriptors []UnixFD) ([]string, *Error) {
	var result []string
	for _, descriptor := range descriptors {
		file := os.NewFile(uintptr(descriptor), "received-fd")
		var data [4]byte
		_, err := file.ReadAt(data[:], 0)
		_ = file.Close()
		if err != nil {
			return nil, MakeFailedError(err)
		}
		result = append(result, string(data[:]))
	}
	return result, nil
}

func (fdContainerService) ReadMap(descriptors map[string]UnixFD) (map[string]string, *Error) {
	result := make(map[string]string)
	for key, descriptor := range descriptors {
		values, err := (fdContainerService{}).ReadArray([]UnixFD{descriptor})
		if err != nil {
			return nil, err
		}
		result[key] = values[0]
	}
	return result, nil
}

func TestFDContainersThroughDaemon(t *testing.T) {
	conn, err := ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.Export(fdContainerService{}, "/org/godbus/FDContainers", "org.godbus.FDContainers"); err != nil {
		t.Fatal(err)
	}
	var descriptors []UnixFD
	for _, text := range []string{"left", "rght"} {
		file, err := os.CreateTemp(t.TempDir(), "descriptor-")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString(text); err != nil {
			t.Fatal(err)
		}
		descriptors = append(descriptors, UnixFD(file.Fd()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	object := conn.Object(conn.Names()[0], "/org/godbus/FDContainers")
	var array []string
	if err := object.CallWithContext(ctx, "org.godbus.FDContainers.ReadArray", 0, descriptors).Store(&array); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(array, []string{"left", "rght"}) {
		t.Fatal(array)
	}
	var dictionary map[string]string
	if err := object.CallWithContext(ctx, "org.godbus.FDContainers.ReadMap", 0, map[string]UnixFD{"a": descriptors[0], "b": descriptors[1]}).Store(&dictionary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dictionary, map[string]string{"a": "left", "b": "rght"}) {
		t.Fatal(dictionary)
	}
}
