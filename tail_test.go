package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
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
