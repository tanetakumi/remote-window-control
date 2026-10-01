// Package testutil holds helpers shared by the tests under control-server/test.
package testutil

import (
	"encoding/binary"

	"share-app-host/internal/capture"
)

// FramePacket builds one CaptureProbe stream frame: the 24-byte header followed
// by width x height BGRA pixels, every payload byte set to byte(id).
func FramePacket(id int64, width, height int) []byte {
	stride := width * 4
	payload := stride * height
	packet := make([]byte, capture.HeaderSize+payload)
	binary.LittleEndian.PutUint32(packet[0:], uint32(payload))
	binary.LittleEndian.PutUint32(packet[4:], uint32(width))
	binary.LittleEndian.PutUint32(packet[8:], uint32(height))
	binary.LittleEndian.PutUint32(packet[12:], uint32(stride))
	binary.LittleEndian.PutUint64(packet[16:], uint64(id))
	for i := capture.HeaderSize; i < len(packet); i++ {
		packet[i] = byte(id)
	}
	return packet
}
