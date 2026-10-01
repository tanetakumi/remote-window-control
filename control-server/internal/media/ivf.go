package media

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"

	pionmedia "github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
)

// consumeIVF reads an IVF stream until EOF and writes each frame to sink.
func consumeIVF(sink SampleWriter, stream io.Reader) error {
	reader, header, err := ivfreader.NewWith(bufio.NewReader(stream))
	if err != nil {
		return err
	}
	if header.TimebaseNumerator == 0 || header.TimebaseDenominator == 0 {
		return fmt.Errorf("invalid IVF timebase")
	}
	duration := time.Second * time.Duration(header.TimebaseNumerator) / time.Duration(header.TimebaseDenominator)
	for {
		payload, _, err := reader.ParseNextFrame()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := sink.WriteSample(pionmedia.Sample{Data: payload, Duration: duration}); err != nil {
			return err
		}
	}
}
