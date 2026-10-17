package dbus

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"
)

// An encoder encodes values to the D-Bus wire format.
type encoder struct {
	out   io.Writer
	fds   []int
	order binary.ByteOrder
	pos   int
	buf   [8]byte
	zero  [8]byte
}

// NewEncoder returns a new encoder that writes to out in the given byte order.
func newEncoder(out io.Writer, order binary.ByteOrder, fds []int) *encoder {
	enc := newEncoderAtOffset(out, 0, order, fds)
	return enc
}

// newEncoderAtOffset returns a new encoder that writes to out in the given
// byte order. Specify the offset to initialize pos for proper alignment
// computation.
func newEncoderAtOffset(out io.Writer, offset int, order binary.ByteOrder, fds []int) *encoder {
	enc := new(encoder)
	enc.out = out
	enc.order = order
	enc.pos = offset
	enc.fds = fds
	return enc
}

// Aligns the next output to be on a multiple of n. Panics on write errors.
func (enc *encoder) align(n int) {
	pad := enc.padding(0, n)
	if pad > 0 {
		enc.write(enc.zero[:pad])
	}
}

// pad returns the number of bytes of padding, based on current position and additional offset.
// and alignment.
func (enc *encoder) padding(offset, algn int) int {
	abs := enc.pos + offset
	if abs%algn != 0 {
		newabs := (abs + algn - 1) & ^(algn - 1)
		return newabs - abs
	}
	return 0
}

// write detects writers that violate the full-write contract as well as errors.
func (enc *encoder) write(data []byte) {
	n, err := enc.out.Write(data)
	enc.pos += n
	if err != nil {
		codecPanic(err)
	}
	if n != len(data) {
		codecPanic(io.ErrShortWrite)
	}
}

func (enc *encoder) writeUint(value uint64, size int) {
	switch size {
	case 1:
		enc.buf[0] = byte(value)
	case 2:
		enc.order.PutUint16(enc.buf[:], uint16(value))
	case 4:
		enc.order.PutUint32(enc.buf[:], uint32(value))
	case 8:
		enc.order.PutUint64(enc.buf[:], value)
	}
	enc.write(enc.buf[:size])
}

func (enc *encoder) writeString(value string) {
	n, err := io.WriteString(enc.out, value)
	enc.pos += n
	if err != nil {
		codecPanic(err)
	}
	if n != len(value) {
		codecPanic(io.ErrShortWrite)
	}
	enc.write(enc.zero[:1])
}

// Encode encodes the given values to the underlying reader. All written values
// are aligned properly as required by the D-Bus spec.
func (enc *encoder) Encode(vs ...any) (err error) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		failure, ok := v.(codecError)
		if !ok {
			// Not ours; re-panic.
			panic(v)
		}
		err = failure.err
	}()
	for _, v := range vs {
		enc.encode(reflect.ValueOf(v), 0)
	}
	return nil
}

// encode encodes the given value to the writer and panics on error. depth holds
// the depth of the container nesting.
func (enc *encoder) encode(v reflect.Value, depth int) {
	if depth > 64 {
		codecPanic(FormatError("input exceeds depth limitation"))
	}
	enc.align(alignment(v.Type()))
	switch v.Kind() {
	case reflect.Uint8:
		enc.writeUint(v.Uint(), 1)
	case reflect.Bool:
		enc.align(4)
		if v.Bool() {
			enc.writeUint(1, 4)
		} else {
			enc.writeUint(0, 4)
		}
	case reflect.Int16:
		enc.writeUint(uint64(v.Int()), 2)
	case reflect.Uint16:
		enc.writeUint(v.Uint(), 2)
	case reflect.Int, reflect.Int32:
		if v.Type() == unixFDType {
			fd := v.Int()
			idx := len(enc.fds)
			enc.fds = append(enc.fds, int(fd))
			enc.writeUint(uint64(idx), 4)
		} else {
			enc.writeUint(uint64(v.Int()), 4)
		}
	case reflect.Uint, reflect.Uint32:
		enc.writeUint(v.Uint(), 4)
	case reflect.Int64:
		enc.writeUint(uint64(v.Int()), 8)
	case reflect.Uint64:
		enc.writeUint(v.Uint(), 8)
	case reflect.Float64:
		enc.writeUint(math.Float64bits(v.Float()), 8)
	case reflect.String:
		str := v.String()
		if !utf8.ValidString(str) {
			codecPanic(FormatError("input has a not-utf8 char in string"))
		}
		if strings.IndexByte(str, byte(0)) != -1 {
			codecPanic(FormatError("input has a null char('\\000') in string"))
		}
		if v.Type() == objectPathType {
			if !ObjectPath(str).IsValid() {
				codecPanic(FormatError("invalid object path"))
			}
		}
		enc.encode(reflect.ValueOf(uint32(len(str))), depth)
		enc.writeString(str)
	case reflect.Pointer:
		enc.encode(v.Elem(), depth)
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			if v.Len() > maxArrayLen {
				codecPanic(FormatError("input exceeds array size limitation"))
			}
			enc.encode(reflect.ValueOf(uint32(v.Len())), depth)
			enc.write(v.Bytes())
			return
		}
		if v.Kind() == reflect.Slice && enc.encodeNumericArray(v, depth) {
			return
		}
		// Lookahead offset: 4 bytes for uint32 length (with alignment),
		// plus alignment for elements.
		n := enc.padding(0, 4) + 4
		offset := enc.pos + n + enc.padding(n, alignment(v.Type().Elem()))

		var buf bytes.Buffer
		bufenc := newEncoderAtOffset(&buf, offset, enc.order, enc.fds)

		for i := 0; i < v.Len(); i++ {
			bufenc.encode(v.Index(i), depth+1)
			if buf.Len() > maxArrayLen {
				codecPanic(FormatError("input exceeds array size limitation"))
			}
		}

		if buf.Len() > maxArrayLen {
			codecPanic(FormatError("input exceeds array size limitation"))
		}

		enc.fds = bufenc.fds
		enc.encode(reflect.ValueOf(uint32(buf.Len())), depth)
		enc.align(alignment(v.Type().Elem()))
		enc.write(buf.Bytes())
	case reflect.Struct:
		switch t := v.Type(); t {
		case signatureType:
			str := v.Field(0)
			if str.Len() > 255 {
				codecPanic(FormatError("signature exceeds size limitation"))
			}
			enc.encode(reflect.ValueOf(byte(str.Len())), depth)
			enc.writeString(str.String())
		case variantType:
			variant := v.Interface().(Variant)
			enc.encode(reflect.ValueOf(variant.sig), depth+1)
			enc.encode(reflect.ValueOf(variant.value), depth+1)
		default:
			for i := 0; i < v.Type().NumField(); i++ {
				field := t.Field(i)
				if field.PkgPath == "" && field.Tag.Get("dbus") != "-" {
					enc.encode(v.Field(i), depth+1)
				}
			}
		}
	case reflect.Map:
		// Maps are arrays of structures, so they actually increase the depth by
		// 2.
		if !isKeyType(v.Type().Key()) {
			codecPanic(InvalidTypeError{v.Type()})
		}
		keys := v.MapKeys()
		// Lookahead offset: 4 bytes for uint32 length (with alignment),
		// plus 8-byte alignment
		n := enc.padding(0, 4) + 4
		offset := enc.pos + n + enc.padding(n, 8)

		var buf bytes.Buffer
		bufenc := newEncoderAtOffset(&buf, offset, enc.order, enc.fds)
		for _, k := range keys {
			bufenc.align(8)
			bufenc.encode(k, depth+2)
			bufenc.encode(v.MapIndex(k), depth+2)
			if buf.Len() > maxArrayLen {
				codecPanic(FormatError("input exceeds array size limitation"))
			}
		}
		enc.fds = bufenc.fds
		enc.encode(reflect.ValueOf(uint32(buf.Len())), depth)
		enc.align(8)
		enc.write(buf.Bytes())
	case reflect.Interface:
		enc.encode(reflect.ValueOf(MakeVariant(v.Interface())), depth)
	default:
		codecPanic(InvalidTypeError{v.Type()})
	}
}
