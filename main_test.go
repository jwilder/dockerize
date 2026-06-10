package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestSliceVarString(t *testing.T) {
	var sv sliceVar
	sv.Set("test1")
	sv.Set("test2")
	result := sv.String()
	assert.Equal(t, "test1,test2", result)
}

func TestHostFlagsVarString(t *testing.T) {
	var hf hostFlagsVar
	hf.Set("host1")
	hf.Set("host2")
	result := hf.String()
	assert.Equal(t, "[host1 host2]", result)
}

func TestExists(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-exists")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	existsResult, err := exists(tempFile.Name())
	assert.NoError(t, err)
	assert.True(t, existsResult)

	nonExisting := "/path/that/does/not/exist"
	existsResult, err = exists(nonExisting)
	assert.NoError(t, err)
	assert.False(t, existsResult)
}

func TestContains(t *testing.T) {
	testMap := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	assert.True(t, contains(testMap, "key1"))
	assert.True(t, contains(testMap, "key2"))
	assert.False(t, contains(testMap, "key3"))
}

func TestDefaultValue(t *testing.T) {
	result, err := defaultValue("test-value")
	assert.NoError(t, err)
	assert.Equal(t, "test-value", result)

	result, err = defaultValue("test-value", "default-value")
	assert.NoError(t, err)
	assert.Equal(t, "test-value", result)

	result, err = defaultValue(nil, "default-value")
	assert.NoError(t, err)
	assert.Equal(t, "default-value", result)

	_, err = defaultValue(nil, nil)
	assert.Error(t, err)

	_, err = defaultValue()
	assert.Error(t, err)
}

func TestParseUrl(t *testing.T) {
	url := parseUrl("http://example.com/path")
	assert.Equal(t, "http", url.Scheme)
	assert.Equal(t, "example.com", url.Host)
	assert.Equal(t, "/path", url.Path)

	url = parseUrl("https://api.example.com:8080/v1/users")
	assert.Equal(t, "https", url.Scheme)
	assert.Equal(t, "api.example.com:8080", url.Host)
	assert.Equal(t, "/v1/users", url.Path)
}

func TestAdd(t *testing.T) {
	result := add(5, 3)
	assert.Equal(t, 8, result)

	result = add(-1, 1)
	assert.Equal(t, 0, result)

	result = add(0, 0)
	assert.Equal(t, 0, result)
}

func TestIsTrue(t *testing.T) {
	assert.True(t, isTrue("true"))
	assert.True(t, isTrue("TRUE"))
	assert.True(t, isTrue("1"))
	assert.True(t, isTrue("yes"))
	assert.True(t, isTrue("on"))

	assert.False(t, isTrue("false"))
	assert.False(t, isTrue("FALSE"))
	assert.False(t, isTrue("0"))
	assert.False(t, isTrue("no"))
	assert.False(t, isTrue("off"))
	assert.False(t, isTrue(""))
	assert.False(t, isTrue("invalid"))
}

func TestLoop(t *testing.T) {
	ch, err := loop(3)
	assert.NoError(t, err)

	var result []int
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{0, 1, 2}, result)

	ch, err = loop(2, 5)
	assert.NoError(t, err)

	result = []int{}
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{2, 3, 4}, result)

	ch, err = loop(0, 10, 2)
	assert.NoError(t, err)

	result = []int{}
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{0, 2, 4, 6, 8}, result)

	_, err = loop()
	assert.Error(t, err)

	_, err = loop(1, 2, 3, 4)
	assert.Error(t, err)
}

func TestWaitForSocketUsesPassedTimeoutForDial(t *testing.T) {
	oldDialTimeout := dialTimeout
	oldTimeout := waitTimeoutFlag
	oldRetry := waitRetryInterval
	wg = sync.WaitGroup{}
	waitTimeoutFlag = 200 * time.Millisecond
	waitRetryInterval = 10 * time.Millisecond
	defer func() {
		dialTimeout = oldDialTimeout
		waitTimeoutFlag = oldTimeout
		waitRetryInterval = oldRetry
		wg = sync.WaitGroup{}
	}()

	timeoutArg := make(chan time.Duration, 1)
	dialTimeout = func(network, address string, timeout time.Duration) (net.Conn, error) {
		timeoutArg <- timeout
		client, server := net.Pipe()
		go server.Close()
		return client, nil
	}

	waitForSocket("tcp", "example:1234", 75*time.Millisecond)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for socket connection")
	}

	select {
	case got := <-timeoutArg:
		assert.Equal(t, 75*time.Millisecond, got)
	case <-time.After(time.Second):
		t.Fatal("dial timeout was not recorded")
	}
}

func TestWaitForSocketLogsDialTarget(t *testing.T) {
	oldDialTimeout := dialTimeout
	oldRetry := waitRetryInterval
	oldOutput := log.Writer()
	wg = sync.WaitGroup{}
	waitRetryInterval = 10 * time.Millisecond
	defer func() {
		dialTimeout = oldDialTimeout
		waitRetryInterval = oldRetry
		log.SetOutput(oldOutput)
		wg = sync.WaitGroup{}
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)

	dialErr := errors.New("dial failed")
	dialDone := make(chan struct{}, 1)
	dialTimeout = func(network, address string, timeout time.Duration) (net.Conn, error) {
		select {
		case dialDone <- struct{}{}:
		default:
		}
		return nil, dialErr
	}

	waitForSocket("tcp", "example:1234", 75*time.Millisecond)

	select {
	case <-dialDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dial attempt")
	}

	assert.Contains(t, strings.TrimSpace(buf.String()), "Problem with dial tcp://example:1234: dial failed")
}

func TestWaitForSocketConnectsToTCPServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer ln.Close()

	oldRetry := waitRetryInterval
	oldTimeout := waitTimeoutFlag
	oldDialTimeout := dialTimeout
	wg = sync.WaitGroup{}
	waitRetryInterval = 10 * time.Millisecond
	waitTimeoutFlag = 200 * time.Millisecond
	defer func() {
		wg = sync.WaitGroup{}
		waitRetryInterval = oldRetry
		waitTimeoutFlag = oldTimeout
		dialTimeout = oldDialTimeout
	}()

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	waitForSocket("tcp", ln.Addr().String(), waitTimeoutFlag)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for socket connection")
	}

	select {
	case <-acceptDone:
	case <-time.After(time.Second):
		t.Fatal("server did not accept test connection")
	}
}

func TestWaitForDependenciesWaitsForFileAndHTTP(t *testing.T) {
	tmpDir := t.TempDir()
	readyFile := filepath.Join(tmpDir, "ready.txt")

	var mu sync.Mutex
	requestCount := 0
	var seenHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requestCount++
		seenHeader = r.Header.Get("X-Test")
		if requestCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	fileURL := url.URL{Scheme: "file", Path: readyFile}

	oldURLs := urls
	oldHeaders := headers
	oldWaitFlag := waitFlag
	oldRetry := waitRetryInterval
	oldTimeout := waitTimeoutFlag
	urls = []url.URL{fileURL, *httpURL}
	headers = []HttpHeader{{name: "X-Test", value: "value"}}
	waitFlag = hostFlagsVar{"file://" + readyFile, server.URL}
	wg = sync.WaitGroup{}
	waitRetryInterval = 10 * time.Millisecond
	waitTimeoutFlag = time.Second
	defer func() {
		urls = oldURLs
		headers = oldHeaders
		waitFlag = oldWaitFlag
		wg = sync.WaitGroup{}
		waitRetryInterval = oldRetry
		waitTimeoutFlag = oldTimeout
	}()

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = os.WriteFile(readyFile, []byte("ready"), 0o644)
	}()

	waitForDependencies()

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, requestCount, 2)
	assert.Equal(t, "value", seenHeader)
}

func TestSignalProcessWithTimeoutPassesSignal(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 10")
	err := cmd.Start()
	assert.NoError(t, err)

	signalProcessWithTimeout(cmd, syscall.SIGTERM)
	assert.NotNil(t, cmd.ProcessState)
	assert.NotNil(t, cmd.Process)
	err = cmd.Process.Signal(syscall.Signal(0))
	assert.Error(t, err)
}

func TestRunCmdCancelsContextWhenCommandExits(t *testing.T) {
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	wg = sync.WaitGroup{}
	defer func() {
		wg = sync.WaitGroup{}
	}()

	wg.Add(1)
	go runCmd(ctx, cancelFn, "sh", "-c", "exit 0")

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("expected command completion to cancel context")
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runCmd goroutines")
	}
}
