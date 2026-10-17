package dbus

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func BenchmarkCodecNumericArray(b *testing.B) {
	for _, size := range []int{16, 4096} {
		data := make([]float64, size)
		for i := range data {
			data[i] = float64(i) / 8
		}
		name := "16"
		if size == 4096 {
			name = "4096"
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(size * 8))
			b.ReportAllocs()
			var buf bytes.Buffer
			for b.Loop() {
				buf.Reset()
				if err := newEncoder(&buf, binary.LittleEndian, nil).Encode(data); err != nil {
					b.Fatal(err)
				}
				if _, err := newDecoder(&buf, binary.LittleEndian, nil).Decode(Signature{"ad"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
