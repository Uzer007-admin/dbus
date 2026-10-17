package dbus

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"reflect"
	"testing"
)

func TestNumericArrayWireAndOwnership(t *testing.T) {
	tests := []struct {
		sig, little, big string
		value            any
	}{
		{"an", "04000000ffffffff", "00000004ffffffff", []int16{-1, -1}},
		{"aq", "0400000034127856", "0000000412345678", []uint16{0x1234, 0x5678}},
		{"ai", "08000000ffffffff00000080", "00000008ffffffff80000000", []int32{-1, -2147483648}},
		{"au", "0800000078563412ffffffff", "0000000812345678ffffffff", []uint32{0x12345678, 0xffffffff}},
		{"ax", "1000000000000000ffffffffffffffff0000000000000080", "0000001000000000ffffffffffffffff8000000000000000", []int64{-1, -9223372036854775808}},
		{"at", "10000000000000000807060504030201ffffffffffffffff", "00000010000000000102030405060708ffffffffffffffff", []uint64{0x0102030405060708, ^uint64(0)}},
		{"ad", "1000000000000000000000000000f83f000000000000f07f", "00000010000000003ff80000000000007ff0000000000000", []float64{1.5, math.Inf(1)}},
	}
	for _, test := range tests {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			wantHex := test.little
			if order == binary.BigEndian {
				wantHex = test.big
			}
			want, err := hex.DecodeString(wantHex)
			if err != nil {
				t.Fatal(err)
			}
			var buffer bytes.Buffer
			if err := newEncoder(&buffer, order, nil).Encode(test.value); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buffer.Bytes(), want) {
				t.Fatalf("%s %s: wire %x, want %x", test.sig, order, buffer.Bytes(), want)
			}
			// Decode an independent wire vector, then reuse the decoder's scratch.
			wire := append(append([]byte{}, want...), bytes.Repeat([]byte{0}, ((-len(want))&7)+8)...)
			dec := newDecoder(bytes.NewReader(wire), order, nil)
			values, err := dec.Decode(Signature{test.sig + "t"})
			if err != nil || !reflect.DeepEqual(values[0], test.value) || values[1] != uint64(0) {
				t.Fatalf("%s %s: %v; %v", test.sig, order, values, err)
			}
		}
	}
}

func TestNumericArrayEmptyAndInvalidLengths(t *testing.T) {
	for _, sig := range []string{"an", "aq", "ai", "au", "ax", "at", "ad"} {
		values, err := newDecoder(bytes.NewReader(make([]byte, 8)), binary.LittleEndian, nil).Decode(Signature{sig})
		if err != nil || reflect.ValueOf(values[0]).IsNil() || reflect.ValueOf(values[0]).Len() != 0 {
			t.Fatalf("%s: %v; %v", sig, values, err)
		}
		for _, length := range []uint32{1, maxArrayLen + 1, maxArrayLen} {
			wire := make([]byte, 4)
			binary.LittleEndian.PutUint32(wire, length)
			if _, err := newDecoder(bytes.NewReader(wire), binary.LittleEndian, nil).Decode(Signature{sig}); err == nil {
				t.Fatalf("%s: malformed/truncated length %d accepted", sig, length)
			}
		}
	}
}

func TestNumericArrayRetainsFDSemanticsAndFloatBits(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		var buffer bytes.Buffer
		enc := newEncoder(&buffer, order, nil)
		bits := []uint64{0x7ff8000000000001, 0x8000000000000000}
		values := []float64{math.Float64frombits(bits[0]), math.Float64frombits(bits[1])}
		if err := enc.Encode([]UnixFD{10, 20}, values); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(enc.fds, []int{10, 20}) {
			t.Fatal(enc.fds)
		}
		decoded, err := newDecoder(&buffer, order, enc.fds).Decode(Signature{"ahad"})
		if err != nil || !reflect.DeepEqual(decoded[0], []UnixFD{10, 20}) {
			t.Fatalf("%v; %v", decoded, err)
		}
		for i, value := range decoded[1].([]float64) {
			if math.Float64bits(value) != bits[i] {
				t.Fatalf("float bits: %x, want %x", math.Float64bits(value), bits[i])
			}
		}
	}
}
