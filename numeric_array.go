package dbus

import (
	"encoding/binary"
	"reflect"
)

// encodeNumericArray batches fixed-width scalar slices. UnixFD and UnixFDIndex
// deliberately keep the regular path: their elements have separate FD semantics.
func (enc *encoder) encodeNumericArray(v reflect.Value, depth int) bool {
	value := v.Interface()
	width := 0
	switch value.(type) {
	case []int16, []uint16:
		width = 2
	case []int32, []uint32:
		width = 4
	case []int64, []uint64, []float64:
		width = 8
	default:
		return false
	}
	if v.Len() > maxArrayLen/width {
		codecPanic(FormatError("input exceeds array size limitation"))
	}
	if depth >= 64 && v.Len() > 0 {
		codecPanic(FormatError("input exceeds depth limitation"))
	}
	data := make([]byte, v.Len()*width)
	if _, err := binary.Encode(data, enc.order, value); err != nil {
		codecPanic(err)
	}
	enc.writeUint(uint64(len(data)), 4)
	enc.align(width)
	enc.write(data)
	return true
}

func (dec *decoder) decodeNumericArray(sig string, length int) (any, bool) {
	width := 0
	switch sig {
	case "n", "q":
		width = 2
	case "i", "u":
		width = 4
	case "x", "t", "d":
		width = 8
	default:
		return nil, false
	}
	if length%width != 0 {
		codecPanic(FormatError("array length does not match element size"))
	}
	dec.align(width) // Also required for empty 64-bit arrays.
	// read2buf grows from actual input rather than allocating an advertised length.
	// Allocate the owned result only after all bytes have been received.
	dec.read2buf(length)
	dec.pos += length
	count := length / width
	var value any
	switch sig {
	case "n":
		value = make([]int16, count)
	case "q":
		value = make([]uint16, count)
	case "i":
		value = make([]int32, count)
	case "u":
		value = make([]uint32, count)
	case "x":
		value = make([]int64, count)
	case "t":
		value = make([]uint64, count)
	case "d":
		value = make([]float64, count)
	}
	if _, err := binary.Decode(dec.buf, dec.order, value); err != nil {
		codecPanic(err)
	}
	return value, true
}
