package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestSliceVarString(t *testing.T) {
	t.Parallel()

	var sv sliceVar
	if err := sv.Set("test1"); err != nil {
		t.Fatal(err)
	}
	if err := sv.Set("test2"); err != nil {
		t.Fatal(err)
	}
	result := sv.String()
	assert.Equal(t, "test1,test2", result)
}

func TestHostFlagsVarString(t *testing.T) {
	t.Parallel()

	var hf hostFlagsVar
	if err := hf.Set("host1"); err != nil {
		t.Fatal(err)
	}
	if err := hf.Set("host2"); err != nil {
		t.Fatal(err)
	}
	result := hf.String()
	assert.Equal(t, "[host1 host2]", result)
}

func TestExists(t *testing.T) {
	t.Parallel()

	tempFile, err := os.CreateTemp("", "test-exists")
	assert.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(tempFile.Name()) })

	existsResult, err := exists(tempFile.Name())
	assert.NoError(t, err)
	assert.True(t, existsResult)

	nonExisting := "/path/that/does/not/exist"
	existsResult, err = exists(nonExisting)
	assert.NoError(t, err)
	assert.False(t, existsResult)
}

func TestContains(t *testing.T) {
	t.Parallel()

	testMap := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	assert.True(t, contains(testMap, "key1"))
	assert.True(t, contains(testMap, "key2"))
	assert.False(t, contains(testMap, "key3"))
}

func TestContextEnvReturnsCurrentEnvironment(t *testing.T) {
	t.Parallel()

	ctx := &Context{}
	env := ctx.Env()
	expected := make(map[string]string)

	for _, entry := range os.Environ() {
		sep := -1
		for i := 0; i < len(entry); i++ {
			if entry[i] == '=' {
				sep = i
				break
			}
		}
		if sep == -1 {
			t.Fatalf("environment entry missing '=': %q", entry)
		}
		expected[entry[:sep]] = entry[sep+1:]
	}

	assert.Equal(t, expected, env)
}

func TestEnvMapSkipsEntriesWithoutEquals(t *testing.T) {
	t.Parallel()

	env := envMap([]string{"A=1", "BADENTRY", "B=x=y", "=empty"})

	// An empty key is kept, as it was before envMap existed.
	assert.Equal(t, map[string]string{"A": "1", "B": "x=y", "": "empty"}, env)
}

func TestEnvMapKeepsEverythingAfterFirstEquals(t *testing.T) {
	t.Parallel()

	env := envMap([]string{"JSON={\"a\":\"b=c\"}", "EMPTY="})

	assert.Equal(t, map[string]string{"JSON": "{\"a\":\"b=c\"}", "EMPTY": ""}, env)
}

func TestDefaultValue(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	url, err := parseUrl("http://example.com/path")
	require.NoError(t, err)
	assert.Equal(t, "http", url.Scheme)
	assert.Equal(t, "example.com", url.Host)
	assert.Equal(t, "/path", url.Path)

	url, err = parseUrl("https://api.example.com:8080/v1/users")
	require.NoError(t, err)
	assert.Equal(t, "https", url.Scheme)
	assert.Equal(t, "api.example.com:8080", url.Host)
	assert.Equal(t, "/v1/users", url.Path)
}

func TestAdd(t *testing.T) {
	t.Parallel()

	result := add(5, 3)
	assert.Equal(t, 8, result)

	result = add(-1, 1)
	assert.Equal(t, 0, result)

	result = add(0, 0)
	assert.Equal(t, 0, result)
}

func TestIsTrue(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func TestParseDelimiters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "empty", input: "", want: nil},
		{name: "valid", input: "{{:}}", want: []string{"{{", "}}"}},
		{name: "valid with spaces", input: "<% : %>", want: []string{"<% ", " %>"}},
		{name: "missing separator", input: "{{}}", wantErr: "bad delimiters argument: {{}}. expected \"left:right\""},
		{name: "too many separators", input: "a:b:c", wantErr: "bad delimiters argument: a:b:c. expected \"left:right\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDelimiters(tt.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseWaitURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   hostFlagsVar
		want    []url.URL
		wantErr string
	}{
		{name: "empty", input: nil, want: []url.URL{}},
		{
			name:  "multiple urls",
			input: hostFlagsVar{"tcp://db:5432", "http://web:8080/health", "file:///tmp/ready"},
			want: []url.URL{
				{Scheme: "tcp", Host: "db:5432"},
				{Scheme: "http", Host: "web:8080", Path: "/health"},
				{Scheme: "file", Path: "/tmp/ready"},
			},
		},
		{name: "invalid escape", input: hostFlagsVar{"http://example.com/%zz"}, wantErr: "bad hostname provided: http://example.com/%zz. parse \"http://example.com/%zz\": invalid URL escape \"%zz\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWaitURLs(tt.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		values  []string
		waits   hostFlagsVar
		want    []HttpHeader
		wantErr string
	}{
		{name: "empty", values: nil, waits: hostFlagsVar{"http://example.com"}, want: []HttpHeader{}},
		{
			name:   "valid headers",
			values: []string{"Accept-Encoding: gzip", "X-Test: value:with:colon", "Authorization:Bearer token"},
			waits:  hostFlagsVar{"http://example.com"},
			want: []HttpHeader{
				{name: "Accept-Encoding", value: "gzip"},
				{name: "X-Test", value: "value:with:colon"},
				{name: "Authorization", value: "Bearer token"},
			},
		},
		{name: "header without wait", values: []string{"Accept: gzip"}, wantErr: "-wait-http-header \"Accept: gzip\" provided with no -wait option"},
		{name: "missing colon", values: []string{"Accept gzip"}, waits: hostFlagsVar{"http://example.com"}, wantErr: "bad HTTP Headers argument: Accept gzip. expected \"headerName: headerValue\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHeaders(tt.values, tt.waits)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProcessTemplatesFileAndDirectoryArguments(t *testing.T) {
	resetGlobals(t)

	fileTemplateDir := t.TempDir()
	filePath := filepath.Join(fileTemplateDir, "source.tmpl")
	fileDest := filepath.Join(fileTemplateDir, "out.txt")
	assert.NoError(t, os.WriteFile(filePath, []byte("hello"), 0o644))

	dirPath := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dirPath, "child.tmpl"), []byte("hello"), 0o644))
	dirDest := t.TempDir()

	processTemplates([]string{filePath + ":" + fileDest, dirPath + ":" + dirDest})

	_, err := os.Stat(fileDest)
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dirDest, "child.tmpl"))
	assert.NoError(t, err)
}

func TestWaitForSocketUsesPassedTimeoutForDial(t *testing.T) {
	resetGlobals(t)
	waitTimeoutFlag = 200 * time.Millisecond
	waitRetryInterval = 10 * time.Millisecond

	timeoutArg := make(chan time.Duration, 1)
	dialTimeout = func(network, address string, timeout time.Duration) (net.Conn, error) {
		timeoutArg <- timeout
		client, server := net.Pipe()
		go func() { _ = server.Close() }()
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

func TestWaitForSocketConnectsToTCPServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer func() { _ = ln.Close() }()

	var logs bytes.Buffer
	oldLogWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(oldLogWriter)

	resetGlobals(t)
	waitRetryInterval = 10 * time.Millisecond
	waitTimeoutFlag = 200 * time.Millisecond

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
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

	assert.Contains(t, logs.String(), "Connected to tcp://"+ln.Addr().String())
}

func TestWaitForFileReturnsWhenFileCreatedAfterDelay(t *testing.T) {
	readyFile := filepath.Join(t.TempDir(), "ready.txt")

	oldRetry := waitRetryInterval
	wg = sync.WaitGroup{}
	waitRetryInterval = 10 * time.Millisecond
	defer func() {
		wg = sync.WaitGroup{}
		waitRetryInterval = oldRetry
	}()

	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(readyFile, []byte("ready"), 0o644)
	}()

	waitForFile(url.URL{Scheme: "file", Path: readyFile})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file to be created")
	}
}

func TestWaitForDependenciesWaitsForFileAndHTTP(t *testing.T) {
	tmpDir := t.TempDir()
	readyFile := filepath.Join(tmpDir, "ready.txt")

	var logs bytes.Buffer
	oldLogWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(oldLogWriter)

	var mu sync.Mutex
	requestCount := 0
	var seenHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requestCount++
		seenHeader = r.Header.Get("X-Test")
		if requestCount == 1 {
			// Make the file dependency ready only once the HTTP dependency has been hit.
			_ = os.WriteFile(readyFile, []byte("ready"), 0o644)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	fileURL := url.URL{Scheme: "file", Path: readyFile}

	resetGlobals(t)
	urls = []url.URL{fileURL, *httpURL}
	headers = []HttpHeader{{name: "X-Test", value: "value"}}
	waitFlag = hostFlagsVar{"file://" + readyFile, server.URL}
	waitRetryInterval = 10 * time.Millisecond
	waitTimeoutFlag = 10 * time.Second

	waitForDependencies()

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, requestCount, 2)
	assert.Equal(t, "value", seenHeader)
	assert.Contains(t, logs.String(), "Received 503 from "+server.URL)
}

func TestRunCmdCancelsContextWhenCommandExits(t *testing.T) {
	resetGlobals(t)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

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
