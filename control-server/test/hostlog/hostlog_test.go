package hostlog_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"share-app-host/internal/hostlog"
)

func TestLogsAreBesideExecutableAndSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "share-host.exe")
	for _, message := range []string{"first run\n", "second run\n"} {
		writer, err := hostlog.Open(exe)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(message)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", "share-host.log"))
	if err != nil || string(data) != "first run\nsecond run\n" {
		t.Fatalf("saved log = %q, error = %v", data, err)
	}
}

func TestRotationKeepsTheNewestBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "share-host.log")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// Seed a full current log and numbered backups, simulating a previous run.
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), hostlog.MaxBytes), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= hostlog.Backups; i++ {
		if err := os.WriteFile(fmt.Sprintf("%s.%d", path, i), []byte(fmt.Sprint(i)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writer, err := hostlog.Open(filepath.Join(dir, "share-host.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Write([]byte("new log\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new log\n" {
		t.Fatalf("current = %q, error = %v", data, err)
	}
	info, err := os.Stat(path + ".1")
	if err != nil || info.Size() != hostlog.MaxBytes {
		t.Fatalf("first backup = %v, error = %v", info, err)
	}
	for i := 2; i <= hostlog.Backups; i++ {
		data, err := os.ReadFile(fmt.Sprintf("%s.%d", path, i))
		if err != nil || string(data) != fmt.Sprint(i-1) {
			t.Fatalf("backup %d = %q, error = %v", i, data, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != hostlog.Backups+1 {
		t.Fatalf("log entries = %v, error = %v", entries, err)
	}
}

func TestOversizedWriteIsBoundedWithoutLosingBytes(t *testing.T) {
	dir := t.TempDir()
	writer, err := hostlog.Open(filepath.Join(dir, "share-host.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	payload := bytes.Repeat([]byte("x"), hostlog.MaxBytes+1)
	if n, err := writer.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	path := filepath.Join(dir, "logs", "share-host.log")
	current, err := os.ReadFile(path)
	if err != nil || string(current) != "x" {
		t.Fatalf("current = %q, error = %v", current, err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil || !bytes.Equal(append(backup, current...), payload) {
		t.Fatalf("rotated write lost bytes: %v", err)
	}
}

// On Windows a log file held open by another process (a viewer following it,
// for instance) cannot be renamed. Logging must carry on in the current file
// and rotate once the obstacle is gone, not stop until the host restarts.
func TestBlockedRotationKeepsLogging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "share-host.log")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), hostlog.MaxBytes), 0600); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory in place of the oldest backup cannot be removed.
	blocker := fmt.Sprintf("%s.%d", path, hostlog.Backups)
	if err := os.MkdirAll(filepath.Join(blocker, "held"), 0700); err != nil {
		t.Fatal(err)
	}
	writer, err := hostlog.Open(filepath.Join(dir, "share-host.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, message := range []string{"blocked 1\n", "blocked 2\n"} {
		if _, err := writer.Write([]byte(message)); err != nil {
			t.Fatalf("Write while rotation is blocked = %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(string(data), "aablocked 1\nblocked 2\n") {
		t.Fatalf("current log does not continue: error = %v", err)
	}

	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("rotated\n")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "rotated\n" {
		t.Fatalf("current = %q, error = %v", data, err)
	}
}

func TestConcurrentWritesPreserveCompleteRecords(t *testing.T) {
	dir := t.TempDir()
	writer, err := hostlog.Open(filepath.Join(dir, "share-host.exe"))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				if _, err := writer.Write([]byte("complete record\n")); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	workers.Wait()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", "share-host.log"))
	if err != nil || string(data) != strings.Repeat("complete record\n", 800) {
		t.Fatalf("concurrent records corrupted: bytes=%d error=%v", len(data), err)
	}
}

func TestUnwritableLogDirectoryIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logs"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := hostlog.Open(filepath.Join(dir, "share-host.exe")); err == nil || !strings.Contains(err.Error(), "create log directory") {
		t.Fatalf("Open error = %v", err)
	}
}
