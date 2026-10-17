package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestTailFileWritesAppendedLines(t *testing.T) {
	tailDir := t.TempDir()
	sourcePath := filepath.Join(tailDir, "source.log")
	destPath := filepath.Join(tailDir, "dest.log")

	if err := os.WriteFile(sourcePath, []byte{}, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	dest, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create dest file: %v", err)
	}
	defer func() {
		if err := dest.Close(); err != nil {
			t.Errorf("close dest file: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go tailFile(ctx, sourcePath, true, dest)

	appended := []string{"first line", "second line"}
	for _, line := range appended {
		f, err := os.OpenFile(sourcePath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open source for append: %v", err)
		}
		if _, err := f.WriteString(line + "\n"); err != nil {
			_ = f.Close()
			t.Fatalf("append source line: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close source file: %v", err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := os.ReadFile(destPath)
		if err != nil {
			cancel()
			wg.Wait()
			t.Fatalf("read dest file: %v", err)
		}
		if strings.Split(strings.TrimSpace(string(got)), "\n")[0] == appended[0] && strings.Contains(string(got), appended[1]) {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			wg.Wait()
			t.Fatalf("timed out waiting for tailed lines, got %q", string(got))
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	wg.Wait()

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read final dest file: %v", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(got)))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan dest file: %v", err)
	}
	if len(lines) != len(appended) {
		t.Fatalf("expected %d tailed lines, got %d (%q)", len(appended), len(lines), string(got))
	}
	for i, want := range appended {
		if lines[i] != want {
			t.Fatalf("line %d mismatch: got %q want %q", i, lines[i], want)
		}
	}
}

func TestTailFileStopsWhenContextCanceled(t *testing.T) {
	tailDir := t.TempDir()
	sourcePath := filepath.Join(tailDir, "source.log")
	destPath := filepath.Join(tailDir, "dest.log")

	if err := os.WriteFile(sourcePath, []byte{}, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	dest, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create dest file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go tailFile(ctx, sourcePath, true, dest)

	time.Sleep(200 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tailFile did not stop after context cancellation")
	}

	if err := dest.Close(); err != nil {
		t.Fatalf("close dest file: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest file: %v", err)
	}
	if string(got) != "" {
		t.Fatalf("expected no tailed output, got %q", string(got))
	}
}

// errWriter is an io.Writer whose every Write fails.
type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestTailFileStopsOnWriteError(t *testing.T) {
	tailDir := t.TempDir()
	sourcePath := filepath.Join(tailDir, "source.log")

	if err := os.WriteFile(sourcePath, []byte{}, 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	var logBuf bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(prevOutput) })

	writeErr := errors.New("destination is closed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg.Add(1)
	go tailFile(ctx, sourcePath, true, errWriter{err: writeErr})

	time.Sleep(200 * time.Millisecond)
	f, err := os.OpenFile(sourcePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open source for append: %v", err)
	}
	if _, err := f.WriteString("line\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append source line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close source file: %v", err)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tailFile did not return after a write error")
	}

	if got := logBuf.String(); !strings.Contains(got, "unable to write tailed line from") || !strings.Contains(got, writeErr.Error()) {
		t.Fatalf("expected write error in log output, got %q", got)
	}
}

func TestTailFilePicksUpFileCreatedLater(t *testing.T) {
	resetGlobals(t)
	logs := captureLog(t)

	tailDir := t.TempDir()
	sourcePath := filepath.Join(tailDir, "later.log")
	dest, destPath := newDestFile(t, tailDir)

	stop := startTailFile(t, sourcePath, false, dest)

	waitForLogContaining(t, logs, "Warning: unable to stat "+sourcePath)

	if err := os.WriteFile(sourcePath, []byte("created late\nappended later\n"), 0o644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	waitForFileContaining(t, destPath, "created late", "appended later")
	stop()
}

func TestTailFileReadsNamedPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes via syscall.Mkfifo are not available on Windows")
	}
	resetGlobals(t)

	tailDir := t.TempDir()
	pipePath := filepath.Join(tailDir, "pipe.log")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatalf("create fifo: %v", err)
	}
	dest, destPath := newDestFile(t, tailDir)

	stop := startTailFile(t, pipePath, false, dest)

	// Opening the write end blocks until tailFile opens the read end, so bound it with a timeout.
	writerCh := make(chan *os.File, 1)
	errCh := make(chan error, 1)
	go func() {
		w, err := os.OpenFile(pipePath, os.O_WRONLY, 0)
		if err != nil {
			errCh <- err
			return
		}
		writerCh <- w
	}()

	var writer *os.File
	select {
	case writer = <-writerCh:
		t.Cleanup(func() { _ = writer.Close() })
	case err := <-errCh:
		t.Fatalf("open fifo for writing: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tailFile to open the fifo for reading")
	}

	if _, err := writer.WriteString("from pipe\nsecond pipe line\n"); err != nil {
		t.Fatalf("write to fifo: %v", err)
	}

	waitForFileContaining(t, destPath, "from pipe", "second pipe line")

	// The tail library blocks reading the pipe until the write end closes, so close it before stopping.
	if err := writer.Close(); err != nil {
		t.Fatalf("close fifo writer: %v", err)
	}
	stop()
}

func TestTailFilePollModeReadsExistingAndAppendedLines(t *testing.T) {
	resetGlobals(t)

	tailDir := t.TempDir()
	sourcePath := filepath.Join(tailDir, "poll.log")
	dest, destPath := newDestFile(t, tailDir)

	if err := os.WriteFile(sourcePath, []byte("existing line\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	stop := startTailFile(t, sourcePath, true, dest)

	f, err := os.OpenFile(sourcePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open source for append: %v", err)
	}
	if _, err := f.WriteString("appended line\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append source line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close source file: %v", err)
	}

	waitForFileContaining(t, destPath, "existing line", "appended line")
	stop()
}

// newDestFile creates a destination file in dir and closes it when the test ends.
func newDestFile(t *testing.T, dir string) (*os.File, string) {
	t.Helper()

	destPath := filepath.Join(dir, "dest.log")
	dest, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create dest file: %v", err)
	}
	t.Cleanup(func() { _ = dest.Close() })
	return dest, destPath
}

// startTailFile runs tailFile in the background. The returned stop func cancels it and fails
// the test if it has not returned within five seconds; it is also called on test cleanup.
func startTailFile(t *testing.T, file string, poll bool, dest *os.File) (stop func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go tailFile(ctx, file, poll, dest)

	stop = func() {
		cancel()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("tailFile did not return after context cancellation")
		}
	}
	t.Cleanup(stop)
	return stop
}

// syncBuffer receives log output from the tailFile goroutine while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog sends the standard logger to a buffer until the test ends.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()

	var buf syncBuffer
	oldOutput := log.Writer()
	log.SetOutput(io.Writer(&buf))
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	return &buf
}

func waitForLogContaining(t *testing.T, logs *syncBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for log %q, got %q", want, logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitForFileContaining(t *testing.T, path string, wants ...string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		all := true
		for _, want := range wants {
			if !strings.Contains(string(got), want) {
				all = false
				break
			}
		}
		if all {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q in %s, got %q", wants, path, string(got))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
