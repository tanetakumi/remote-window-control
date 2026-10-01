package capture_test

import (
	"encoding/binary"
	"testing"

	"share-app-host/internal/capture"
)

// repeatFrameReader endlessly yields the same 1080p frame with increasing ids.
type repeatFrameReader struct {
	packet []byte
	offset int
	id     uint64
}

func (r *repeatFrameReader) Read(p []byte) (int, error) {
	if r.offset == 0 {
		r.id++
		binary.LittleEndian.PutUint64(r.packet[16:], r.id)
	}
	n := copy(p, r.packet[r.offset:])
	r.offset = (r.offset + n) % len(r.packet)
	return n, nil
}

// BenchmarkReadInto shows what buffer reuse saves per 1080p frame.
func BenchmarkReadInto(b *testing.B) {
	const (
		width, height = 1920, 1080
		payload       = width * height * 4
	)
	for _, reuse := range []bool{false, true} {
		name := "allocate"
		if reuse {
			name = "reuse"
		}
		b.Run(name, func(b *testing.B) {
			packet := make([]byte, capture.HeaderSize+payload)
			binary.LittleEndian.PutUint32(packet[0:], payload)
			binary.LittleEndian.PutUint32(packet[4:], width)
			binary.LittleEndian.PutUint32(packet[8:], height)
			binary.LittleEndian.PutUint32(packet[12:], width*4)
			r := capture.NewFrameReader(&repeatFrameReader{packet: packet})

			var buffer []byte
			if reuse {
				buffer = make([]byte, payload)
			}
			b.ReportAllocs()
			b.SetBytes(payload)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				frame, err := r.ReadInto(buffer)
				if err != nil {
					b.Fatal(err)
				}
				if reuse {
					buffer = frame.Data
				}
			}
		})
	}
}
