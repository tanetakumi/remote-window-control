// Package hostlog persists host diagnostics beside the running executable.
package hostlog

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	MaxBytes = 5 * 1024 * 1024
	Backups  = 2
)

// Writer keeps share-host.log and two numbered backups, each at most 5 MiB
// unless rotation is blocked (see rotate).
// Writes and rotation are serialized, including writes from subprocesses.
type Writer struct {
	mu   sync.Mutex
	file *os.File
	path string
	size int64
	// rotateFailed is set while rotation keeps failing, so the failure is
	// reported once rather than on every write.
	rotateFailed bool
}

// Open creates logs beside executablePath, independently of the working directory.
// Existing logs are appended to, so restarting the host preserves diagnostics.
func Open(executablePath string) (*Writer, error) {
	dir := filepath.Join(filepath.Dir(executablePath), "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", dir, err)
	}
	w := &Writer{path: filepath.Join(dir, "share-host.log")}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open log %q: %w", w.path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file, w.size = f, info.Size()
	return nil
}

// rotate shifts the backups and starts a new log. If shifting fails, for
// example because another process holds a log file open on Windows, the
// current log is reopened so logging continues; rotation is retried on the
// next write.
func (w *Writer) rotate() error {
	closeErr := w.file.Close()
	w.file = nil
	shiftErr := w.shiftBackups()
	return errors.Join(closeErr, shiftErr, w.open())
}

func (w *Writer) shiftBackups() error {
	if err := os.Remove(fmt.Sprintf("%s.%d", w.path, Backups)); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := Backups - 1; i >= 0; i-- {
		source := w.path
		if i > 0 {
			source = fmt.Sprintf("%s.%d", w.path, i)
		}
		if err := os.Rename(source, fmt.Sprintf("%s.%d", w.path, i+1)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer func() {
		if err != nil {
			// log.Logger ignores output errors. Keep failures visible on the console.
			fmt.Fprintf(os.Stderr, "host log write failed: %v\n", err)
		}
	}()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	for len(p) > 0 {
		if w.size > 0 && w.size+int64(len(p)) > MaxBytes {
			rotateErr := w.rotate()
			if w.file == nil {
				return n, rotateErr
			}
			if rotateErr != nil && !w.rotateFailed {
				fmt.Fprintf(os.Stderr, "host log rotation failed, appending to the current log: %v\n", rotateErr)
			}
			w.rotateFailed = rotateErr != nil
		}
		count := min(len(p), MaxBytes)
		written, writeErr := w.file.Write(p[:count])
		w.size += int64(written)
		n += written
		if writeErr != nil {
			return n, writeErr
		}
		if written != count {
			return n, io.ErrShortWrite
		}
		p = p[count:]
	}
	return n, nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Stderr copies helper diagnostics into the host log as they arrive. The
// caller can also retain a bounded tail for errors sent to the browser.
type Stderr struct{ Label string }

func (s Stderr) Write(p []byte) (int, error) {
	if detail := strings.TrimSpace(string(p)); detail != "" {
		log.Printf("%s stderr=%q", s.Label, detail)
	}
	return len(p), nil
}
