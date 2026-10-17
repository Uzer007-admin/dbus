package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"testing"
)

type zeroThenReader struct {
	io.Reader
	zero bool
}

func (r *zeroThenReader) Read(p []byte) (int, error) {
	if !r.zero {
		r.zero = true
		return 0, nil
	}
	return r.Reader.Read(p)
}

type shortCodecWriter struct{}

type errorPanicIO struct{ err error }

func (r errorPanicIO) Read([]byte) (int, error)  { panic(r.err) }
func (r errorPanicIO) Write([]byte) (int, error) { panic(r.err) }

func TestCodecPreservesForeignPanic(t *testing.T) {
	for _, direction := range []string{"encode", "decode"} {
		t.Run(direction, func(t *testing.T) {
			failure := errors.New("caller panic")
			transport := errorPanicIO{failure}
			defer func() {
				if value := recover(); value != failure {
					t.Fatalf("panic replaced: %v", value)
				}
			}()
			if direction == "encode" {
				_ = newEncoder(transport, binary.LittleEndian, nil).Encode(uint32(1))
			} else {
				_, _ = newDecoder(transport, binary.LittleEndian, nil).Decode(Signature{"y"})
			}
		})
	}
}

func (shortCodecWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestCodecShortWrites(t *testing.T) {
	for _, value := range []any{byte(1), uint16(2), uint32(3), uint64(4), true, 1.5, "value", Signature{"su"}, []byte{1, 2}, []uint32{1, 2}, map[string]byte{"a": 1}} {
		if err := newEncoder(shortCodecWriter{}, binary.LittleEndian, nil).Encode(value); err != io.ErrShortWrite {
			t.Errorf("%T: %v", value, err)
		}
	}
}

func TestCodecZeroRead(t *testing.T) {
	r := &zeroThenReader{Reader: bytes.NewReader([]byte{42})}
	values, err := newDecoder(r, binary.LittleEndian, nil).Decode(Signature{"y"})
	if err != nil || !reflect.DeepEqual(values, []any{byte(42)}) {
		t.Fatalf("%v %v", values, err)
	}
}

func TestCodecInvalidWire(t *testing.T) {
	cases := []struct {
		signature string
		data      []byte
	}{
		{"s", []byte{1, 0, 0, 0, 'A', 'X'}},  // missing terminator
		{"s", []byte{1, 0, 0, 0, 0xff, 0}},   // invalid UTF-8
		{"s", []byte{2, 0, 0, 0, 'A', 0, 0}}, // embedded NUL
		{"o", []byte{1, 0, 0, 0, 'A', 0}},    // invalid object path
		{"g", []byte{1, 's', 'X'}},
		{"yu", []byte{1, 1, 0, 0, 0, 0, 0, 0}},          // nonzero padding
		{"au", []byte{1, 0, 0, 0, 1, 0, 0, 0}},          // partial array element
		{"a{yy}", []byte{1, 0, 0, 0, 0, 0, 0, 0, 1, 2}}, // entry exceeds declared length
	}
	for _, tc := range cases {
		if _, err := newDecoder(bytes.NewReader(tc.data), binary.LittleEndian, nil).Decode(Signature{tc.signature}); err == nil {
			t.Errorf("accepted %s %x", tc.signature, tc.data)
		}
	}
}

func TestCodecByteArrayOwnership(t *testing.T) {
	type namedByte byte
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, source := range []any{[]byte{}, []byte{0, 128, 255}, []namedByte{0, 128, 255}} {
			var buf bytes.Buffer
			if err := newEncoder(&buf, order, nil).Encode(source, []byte{3, 4}, "tail"); err != nil {
				t.Fatal(err)
			}
			values, err := newDecoder(&buf, order, nil).Decode(Signature{"ayays"})
			if err != nil {
				t.Fatal(err)
			}
			want := reflect.ValueOf(source).Bytes()
			if !bytes.Equal(values[0].([]byte), want) || !bytes.Equal(values[1].([]byte), []byte{3, 4}) || values[2] != "tail" {
				t.Fatalf("ownership/round trip: %v", values)
			}
		}
	}
}

func TestCodecScalarGolden(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		var buf bytes.Buffer
		if err := newEncoder(&buf, order, nil).Encode(byte(7), uint16(0x1234), uint32(0x55667788), float64(1.5)); err != nil {
			t.Fatal(err)
		}
		want := []byte{7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		order.PutUint16(want[2:4], 0x1234)
		order.PutUint32(want[4:8], 0x55667788)
		order.PutUint64(want[8:], 0x3ff8000000000000)
		if !bytes.Equal(buf.Bytes(), want) {
			t.Fatalf("wire: %x, want %x", buf.Bytes(), want)
		}
	}
}

func TestCodecBooleanAlignment(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		var buf bytes.Buffer
		if err := newEncoder(&buf, order, nil).Encode(byte(7), true); err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 8)
		want[0] = 7
		order.PutUint32(want[4:], 1)
		if !bytes.Equal(buf.Bytes(), want) {
			t.Fatalf("%x != %x", buf.Bytes(), want)
		}
	}
}

func TestMessageLengthOverflow(t *testing.T) {
	data := make([]byte, 16)
	data[0] = 'l'
	data[1] = byte(TypeMethodReply)
	data[3] = 1
	binary.LittleEndian.PutUint32(data[4:], 0xfffffff0)
	binary.LittleEndian.PutUint32(data[12:], 32)
	if _, err := DecodeMessage(bytes.NewReader(data)); err != InvalidMessageError("message is too long") {
		t.Fatalf("got %v", err)
	}
}

func TestCodecFDIndexWidth(t *testing.T) {
	for _, idx := range []uint32{0, 0x80000000, 0xffffffff} {
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, idx)
		values, err := newDecoder(bytes.NewReader(data), binary.LittleEndian, []int{42}).Decode(Signature{"h"})
		if idx == 0 {
			if err != nil || values[0] != UnixFD(42) {
				t.Fatalf("%v; %v", values, err)
			}
		} else if err == nil {
			t.Fatal("out-of-range FD index accepted")
		}
		// A standalone decoder without ancillary descriptors preserves wire indices.
		values, err = newDecoder(bytes.NewReader(data), binary.LittleEndian, nil).Decode(Signature{"h"})
		if err != nil || values[0] != UnixFDIndex(idx) {
			t.Fatalf("%v; %v", values, err)
		}
	}
}

func BenchmarkCodecByteArray(b *testing.B) {
	data := bytes.Repeat([]byte{42}, 35<<10)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		enc := newEncoder(&buf, binary.LittleEndian, nil)
		if err := enc.Encode(data); err != nil {
			b.Fatal(err)
		}
		if _, err := newDecoder(&buf, binary.LittleEndian, nil).Decode(Signature{"ay"}); err != nil {
			b.Fatal(err)
		}
	}
}
