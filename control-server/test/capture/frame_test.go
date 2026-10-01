package capture_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"share-app-host/internal/capture"
	"share-app-host/test/testutil"
)

func TestReadIntoReusesTheCallerBufferAndRejectsDuplicateIDs(t *testing.T) {
	stream := bytes.Join([][]byte{
		testutil.FramePacket(1, 2, 2),
		testutil.FramePacket(2, 2, 2),
		testutil.FramePacket(2, 2, 2),
	}, nil)
	r := capture.NewFrameReader(bytes.NewReader(stream))
	buffer := make([]byte, 16)

	first, err := r.ReadInto(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if &first.Data[0] != &buffer[0] {
		t.Fatal("allocated instead of reusing the buffer")
	}
	if first.Width != 2 || first.Height != 2 || first.Stride != 8 || first.ID != 1 {
		t.Fatalf("frame metadata: %+v", first)
	}

	second, err := r.ReadInto(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != 2 || second.Data[0] != 2 {
		t.Fatal("corrupt second frame")
	}
	if _, err = r.ReadInto(buffer); !errors.Is(err, capture.ErrInvalidHeader) {
		t.Fatalf("duplicate frame id: error = %v", err)
	}
}

func TestReadIntoAdaptsTheBufferToTheFrameSize(t *testing.T) {
	r := capture.NewFrameReader(bytes.NewReader(bytes.Join([][]byte{
		testutil.FramePacket(1, 2, 2), // 16 bytes
		testutil.FramePacket(2, 4, 4), // 64 bytes
		testutil.FramePacket(3, 2, 2), // 16 bytes
	}, nil)))

	big := make([]byte, 128)
	frame, err := r.ReadInto(big)
	if err != nil || len(frame.Data) != 16 || &frame.Data[0] != &big[0] {
		t.Fatalf("oversized buffer should be resliced and reused: len=%d err=%v", len(frame.Data), err)
	}

	small := make([]byte, 8)
	frame, err = r.ReadInto(small)
	if err != nil || len(frame.Data) != 64 {
		t.Fatalf("undersized buffer should be replaced: len=%d err=%v", len(frame.Data), err)
	}

	frame, err = r.ReadInto(nil)
	if err != nil || len(frame.Data) != 16 {
		t.Fatalf("nil buffer should be allocated: len=%d err=%v", len(frame.Data), err)
	}
}

func TestReadIntoRejectsMalformedHeadersBeforeReadingThePayload(t *testing.T) {
	const (
		lengthField = 0
		widthField  = 4
		heightField = 8
		strideField = 12
		idField     = 16
	)
	set32 := func(field int, v uint32) func([]byte) {
		return func(h []byte) { binary.LittleEndian.PutUint32(h[field:], v) }
	}
	tests := []struct {
		name   string
		mutate func(header []byte)
	}{
		{"length garbage", set32(lengthField, 0xffffffff)},
		{"length not stride*height", set32(lengthField, 15)},
		{"zero width", set32(widthField, 0)},
		{"negative width", set32(widthField, 0xffffffff)},
		{"width over limit", set32(widthField, 16385)},
		{"zero height", set32(heightField, 0)},
		{"height over limit", set32(heightField, 16385)},
		{"stride not 4*width", set32(strideField, 7)},
		{"id zero", func(h []byte) { binary.LittleEndian.PutUint64(h[idField:], 0) }},
		{"id garbage", func(h []byte) { binary.LittleEndian.PutUint64(h[idField:], ^uint64(0)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := testutil.FramePacket(1, 2, 2)[:capture.HeaderSize]
			tt.mutate(header)
			// Only the header is supplied: an accepted header would fail with
			// a short read of the payload instead of ErrInvalidHeader.
			_, err := capture.NewFrameReader(bytes.NewReader(header)).ReadInto(nil)
			if !errors.Is(err, capture.ErrInvalidHeader) {
				t.Fatalf("error = %v, want ErrInvalidHeader", err)
			}
		})
	}
}

func TestReadIntoRejectsFramesAboveTheSizeLimit(t *testing.T) {
	// 16384 x 16384 x 4 bytes = 1 GiB. Every field is individually valid and
	// consistent, so only the 128 MiB frame limit can reject it.
	header := make([]byte, capture.HeaderSize)
	binary.LittleEndian.PutUint32(header[0:], 1<<30)
	binary.LittleEndian.PutUint32(header[4:], 16384)
	binary.LittleEndian.PutUint32(header[8:], 16384)
	binary.LittleEndian.PutUint32(header[12:], 16384*4)
	binary.LittleEndian.PutUint64(header[16:], 1)
	if _, err := capture.NewFrameReader(bytes.NewReader(header)).ReadInto(nil); !errors.Is(err, capture.ErrInvalidHeader) {
		t.Fatalf("error = %v, want ErrInvalidHeader", err)
	}
}

func TestReadIntoReportsTruncatedInput(t *testing.T) {
	packet := testutil.FramePacket(1, 2, 2)
	for name, truncated := range map[string][]byte{
		"empty":              nil,
		"partial header":     packet[:10],
		"partial payload":    packet[:capture.HeaderSize+3],
		"header, no payload": packet[:capture.HeaderSize],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := capture.NewFrameReader(bytes.NewReader(truncated)).ReadInto(nil)
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("error = %v, want an EOF error", err)
			}
		})
	}
}
