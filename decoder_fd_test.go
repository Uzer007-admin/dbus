package dbus

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestFDContainerTypes(t *testing.T) {
	for _, value := range []any{
		[]UnixFD{10, 20},
		[][]UnixFD{{10}, {20}},
		map[string]UnixFD{"fd": 10},
		map[UnixFD]string{10: "fd"},
		MakeVariant([]UnixFD{10, 20}),
	} {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			var buffer bytes.Buffer
			enc := newEncoder(&buffer, order, nil)
			if err := enc.Encode(value); err != nil {
				t.Fatal(err)
			}
			values, err := newDecoder(&buffer, order, enc.fds).Decode(SignatureOf(value))
			if err != nil || !reflect.DeepEqual(values[0], value) {
				t.Fatalf("%T %s: %v; %v", value, order, values, err)
			}
		}
	}
}

func TestInvalidFDContainerIndexReturnsError(t *testing.T) {
	for _, value := range []any{[]UnixFDIndex{0, 1}, map[string]UnixFDIndex{"invalid": 1}, MakeVariant([]UnixFDIndex{1})} {
		var buffer bytes.Buffer
		if err := newEncoder(&buffer, binary.LittleEndian, nil).Encode(value); err != nil {
			t.Fatal(err)
		}
		if _, err := newDecoder(&buffer, binary.LittleEndian, []int{42}).Decode(SignatureOf(value)); err == nil {
			t.Fatalf("%T: out-of-range FD index accepted", value)
		}
	}
}
