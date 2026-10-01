package capture

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

// HeaderSize is the size of the fixed header that precedes each frame on the
// stream protocol spoken by CaptureProbe.
//
// The header is little-endian: payload length (uint32), width, height and
// stride (int32 each), then a strictly increasing frame id (uint64). The
// payload is stride*height bytes of BGRA pixels.
const HeaderSize = 24

const (
	maxFrameBytes = 128 * 1024 * 1024
	maxDimension  = 16384
	bytesPerPixel = 4
)

// ErrInvalidHeader is returned for a malformed or out-of-sequence frame header.
var ErrInvalidHeader = errors.New("invalid capture frame header")

// Frame is one captured BGRA image.
type Frame struct {
	Width, Height, Stride int
	// ID increases with every frame from a stream.
	ID int64
	// Data holds the pixels. When the frame was read into a caller-supplied
	// buffer it aliases that buffer.
	Data []byte
}

// FrameReader reads frames from a CaptureProbe stream. It validates every
// header before allocating or reading the payload, so a corrupt stream cannot
// trigger a huge allocation. It has a single reader and is not safe for
// concurrent use.
type FrameReader struct {
	r      *bufio.Reader
	header [HeaderSize]byte
	lastID int64
}

// NewFrameReader returns a FrameReader reading from r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReader(r)}
}

// ReadInto reads the next frame, reusing buffer for the pixels when it is
// large enough. The caller owns Data until it hands the buffer back, and must
// not reuse it while another goroutine (such as the encoder) still reads it.
func (f *FrameReader) ReadInto(buffer []byte) (Frame, error) {
	if _, err := io.ReadFull(f.r, f.header[:]); err != nil {
		return Frame{}, err
	}
	frame, length, err := decodeHeader(f.header[:], f.lastID)
	if err != nil {
		return Frame{}, err
	}
	if cap(buffer) < length {
		buffer = make([]byte, length)
	} else {
		buffer = buffer[:length]
	}
	if _, err := io.ReadFull(f.r, buffer); err != nil {
		return Frame{}, err
	}
	f.lastID = frame.ID
	frame.Data = buffer
	return frame, nil
}

// decodeHeader validates a header and returns the frame metadata and payload
// length. lastID is the id of the previous frame; ids must strictly increase.
func decodeHeader(header []byte, lastID int64) (Frame, int, error) {
	if len(header) != HeaderSize {
		return Frame{}, 0, ErrInvalidHeader
	}
	length := uint64(binary.LittleEndian.Uint32(header[0:4]))
	frame := Frame{
		Width:  int(int32(binary.LittleEndian.Uint32(header[4:8]))),
		Height: int(int32(binary.LittleEndian.Uint32(header[8:12]))),
		Stride: int(int32(binary.LittleEndian.Uint32(header[12:16]))),
		ID:     int64(binary.LittleEndian.Uint64(header[16:24])),
	}
	valid := frame.Width > 0 && frame.Width <= maxDimension &&
		frame.Height > 0 && frame.Height <= maxDimension &&
		frame.Stride == frame.Width*bytesPerPixel &&
		length == uint64(frame.Stride)*uint64(frame.Height) &&
		length <= maxFrameBytes &&
		frame.ID > lastID
	if !valid {
		return Frame{}, 0, ErrInvalidHeader
	}
	return frame, int(length), nil
}
