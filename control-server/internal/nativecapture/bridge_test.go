package nativecapture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func framePacket(id int64) []byte {
	p := make([]byte, 24+16)
	binary.LittleEndian.PutUint32(p, 16)
	binary.LittleEndian.PutUint32(p[4:], 2)
	binary.LittleEndian.PutUint32(p[8:], 2)
	binary.LittleEndian.PutUint32(p[12:], 8)
	binary.LittleEndian.PutUint64(p[16:], uint64(id))
	for i := 24; i < len(p); i++ {
		p[i] = byte(id)
	}
	return p
}
func TestReadFramesReuseCallerBufferAndRejectDuplicate(t *testing.T) {
	packet := append(framePacket(1), framePacket(2)...)
	packet = append(packet, framePacket(2)...)
	s := &StreamSession{stdout: bufio.NewReader(bytes.NewReader(packet))}
	buffer := make([]byte, 16)
	first, err := s.ReadFrameInto(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if &first.Data[0] != &buffer[0] {
		t.Fatal("allocated instead of reusing buffer")
	}
	second, err := s.ReadFrameInto(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if second.FrameID != 2 || second.Data[0] != 2 {
		t.Fatal("corrupt second frame")
	}
	if _, err = s.ReadFrameInto(buffer); err == nil {
		t.Fatal("duplicate frame accepted")
	}
}
func TestRejectMalformedFrameHeadersBeforeAllocation(t *testing.T) {
	for _, field := range []int{0, 4, 8, 12, 16} {
		header := framePacket(1)[:24]
		if field == 16 {
			binary.LittleEndian.PutUint64(header[field:], ^uint64(0))
		} else {
			binary.LittleEndian.PutUint32(header[field:], 0xffffffff)
		}
		if _, _, err := decodeFrameHeader(header, 0); err == nil {
			t.Errorf("invalid field %d accepted", field)
		}
	}
	s := &StreamSession{stdout: bufio.NewReader(bytes.NewReader(framePacket(1)[:30]))}
	if _, err := s.ReadFrameInto(nil); err == nil {
		t.Fatal("truncated payload accepted")
	}
}
func TestCaptureHelper(t *testing.T) {
	if os.Getenv("RWC_CAPTURE_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}
func TestCloseUnblocksStalledReadAndIsConcurrentSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCaptureHelper$")
	cmd.Env = append(os.Environ(), "RWC_CAPTURE_HELPER=1")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &StreamSession{command: cmd, stdout: bufio.NewReader(pipe), pipe: pipe, cancel: cancel, done: make(chan struct{})}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { s.waitErr = cmd.Wait(); close(s.done) }()
	readDone := make(chan error, 1)
	go func() { _, err := s.ReadFrameInto(nil); readDone <- err }()
	var closers sync.WaitGroup
	for i := 0; i < 3; i++ {
		closers.Add(1)
		go func() { defer closers.Done(); _ = s.Close() }()
	}
	closers.Wait()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("stalled read returned without error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stalled reader survived close")
	}
}

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
func BenchmarkReadFrameInto(b *testing.B) {
	for _, reuse := range []bool{false, true} {
		label := "allocate"
		if reuse {
			label = "reuse"
		}
		b.Run(label, func(b *testing.B) {
			const payload = 1920 * 1080 * 4
			packet := make([]byte, 24+payload)
			binary.LittleEndian.PutUint32(packet, payload)
			binary.LittleEndian.PutUint32(packet[4:], 1920)
			binary.LittleEndian.PutUint32(packet[8:], 1080)
			binary.LittleEndian.PutUint32(packet[12:], 1920*4)
			s := &StreamSession{stdout: bufio.NewReader(&repeatFrameReader{packet: packet})}
			var buffer []byte
			if reuse {
				buffer = make([]byte, payload)
			}
			b.ReportAllocs()
			b.SetBytes(payload)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				frame, err := s.ReadFrameInto(buffer)
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

var _ io.Reader = (*repeatFrameReader)(nil)
