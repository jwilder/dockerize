package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseFlagsForTest parses args into a fresh flag set and returns the resulting Config.
// The slice and host flags accumulate across parses, so they are cleared first.
func parseFlagsForTest(t *testing.T, args []string) (Config, error) {
	t.Helper()
	resetGlobals(t)

	templatesFlag, stdoutTailFlag, stderrTailFlag = nil, nil, nil
	headersFlag, waitFlag, delimsFlag = nil, nil, ""

	oldCommandLine, oldUsage := flag.CommandLine, flag.Usage
	t.Cleanup(func() {
		flag.CommandLine, flag.Usage = oldCommandLine, oldUsage
	})
	flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)

	registerFlags()
	require.NoError(t, flag.CommandLine.Parse(args))
	return parseConfigFromFlags()
}

func urlStrings(urls []url.URL) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		out = append(out, u.String())
	}
	return out
}

func TestParseConfigFromFlags(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{})
		require.NoError(t, err)

		assert.False(t, cfg.version)
		assert.False(t, cfg.poll)
		assert.False(t, cfg.noOverwrite)
		assert.Equal(t, 10*time.Second, cfg.waitTimeout)
		assert.Equal(t, defaultWaitRetryInterval, cfg.waitRetryInterval)
		assert.Empty(t, cfg.templates)
		assert.Empty(t, cfg.stdoutTails)
		assert.Empty(t, cfg.stderrTails)
		assert.Empty(t, cfg.delims)
		assert.Empty(t, cfg.headers)
		assert.Empty(t, cfg.urls)
		assert.Empty(t, cfg.waits)
		assert.Empty(t, cfg.args)
	})

	t.Run("full flag set", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{
			"-template", "a.tmpl:/etc/a.conf",
			"-template", "dir:/etc/conf.d",
			"-wait", "tcp://db:5432",
			"-wait", "http://web:8000/health",
			"-wait", "file:///tmp/ready",
			"-wait-http-header", "Accept-Encoding: gzip",
			"-wait-http-header", "X-Token:abc",
			"-delims", "<%:%>",
			"-timeout", "5s",
			"-wait-retry-interval", "2s",
			"-no-overwrite",
			"-poll",
			"-stdout", "/var/log/a.log",
			"-stdout", "/var/log/b.log",
			"-stderr", "/var/log/err.log",
			"nginx", "-g", "daemon off;",
		})
		require.NoError(t, err)

		assert.True(t, cfg.poll)
		assert.True(t, cfg.noOverwrite)
		assert.False(t, cfg.version)
		assert.Equal(t, []string{"a.tmpl:/etc/a.conf", "dir:/etc/conf.d"}, cfg.templates)
		assert.Equal(t, []string{"/var/log/a.log", "/var/log/b.log"}, cfg.stdoutTails)
		assert.Equal(t, []string{"/var/log/err.log"}, cfg.stderrTails)
		assert.Equal(t, []string{"Accept-Encoding: gzip", "X-Token:abc"}, cfg.headersFlag)
		assert.Equal(t, []string{"<%", "%>"}, cfg.delims)
		assert.Equal(t, []HttpHeader{
			{name: "Accept-Encoding", value: "gzip"},
			{name: "X-Token", value: "abc"},
		}, cfg.headers)
		assert.Equal(t, []string{"tcp://db:5432", "http://web:8000/health", "file:///tmp/ready"}, cfg.waits)
		assert.Equal(t, []string{"tcp://db:5432", "http://web:8000/health", "file:///tmp/ready"}, urlStrings(cfg.urls))
		assert.Equal(t, "tcp", cfg.urls[0].Scheme)
		assert.Equal(t, "http", cfg.urls[1].Scheme)
		assert.Equal(t, "file", cfg.urls[2].Scheme)
		assert.Equal(t, 5*time.Second, cfg.waitTimeout)
		assert.Equal(t, 2*time.Second, cfg.waitRetryInterval)
		assert.Equal(t, []string{"nginx", "-g", "daemon off;"}, cfg.args)
	})

	t.Run("version flag", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{"-version"})
		require.NoError(t, err)
		assert.True(t, cfg.version)
	})

	t.Run("bad delimiters", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{"-delims", "a:b:c"})
		assert.EqualError(t, err, `bad delimiters argument: a:b:c. expected "left:right"`)
		assert.Equal(t, Config{}, cfg)
	})

	t.Run("header without wait", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{"-wait-http-header", "Accept: text/html"})
		assert.ErrorContains(t, err, `-wait-http-header "Accept: text/html" provided with no -wait option`)
		assert.Equal(t, Config{}, cfg)
	})

	t.Run("header without colon", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{"-wait", "tcp://db:5432", "-wait-http-header", "Accept"})
		assert.ErrorContains(t, err, `bad HTTP Headers argument: Accept.`)
		assert.Equal(t, Config{}, cfg)
	})

	t.Run("bad wait URL", func(t *testing.T) {
		cfg, err := parseFlagsForTest(t, []string{"-wait", "http://[::1"})
		assert.ErrorContains(t, err, "bad hostname provided: http://[::1.")
		assert.Equal(t, Config{}, cfg)
	})
}

func TestParseConfigFromFlagsWaitRetryInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     time.Duration
		wantErr  string
	}{
		{name: "zero is rejected", interval: "0", wantErr: "-wait-retry-interval must be positive, got 0s"},
		{name: "negative is rejected", interval: "-1s", wantErr: "-wait-retry-interval must be positive, got -1s"},
		{name: "one nanosecond is accepted", interval: "1ns", want: time.Nanosecond},
		{name: "one second is accepted", interval: "1s", want: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseFlagsForTest(t, []string{"-wait-retry-interval", tt.interval})
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, Config{}, cfg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.waitRetryInterval)
		})
	}
}

func TestParseConfigFromFlagsRestoresCommandLine(t *testing.T) {
	before := flag.CommandLine
	t.Run("parse", func(t *testing.T) {
		_, err := parseFlagsForTest(t, []string{})
		require.NoError(t, err)
	})
	assert.Same(t, before, flag.CommandLine)
}

func TestStartCommandRunsCommand(t *testing.T) {
	resetGlobals(t)
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = oldCommandLine })
	flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
	require.NoError(t, flag.CommandLine.Parse([]string{"true"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startCommand(ctx, cancel)

	waitOrFail(t, "command did not finish")
	assert.Error(t, ctx.Err(), "context should be cancelled once the command exits")
}

func TestStartCommandWithoutArgsAddsNothing(t *testing.T) {
	resetGlobals(t)
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = oldCommandLine })
	flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
	require.NoError(t, flag.CommandLine.Parse([]string{}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startCommand(ctx, cancel)

	waitOrFail(t, "wait group should be empty")
	assert.NoError(t, ctx.Err(), "context should not be cancelled")
}

func TestStartTailersStopsWhenContextCanceled(t *testing.T) {
	resetGlobals(t)
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	require.NoError(t, os.WriteFile(stdoutPath, []byte{}, 0o644))
	require.NoError(t, os.WriteFile(stderrPath, []byte{}, 0o644))

	stdoutTailFlag = sliceVar{stdoutPath}
	stderrTailFlag = sliceVar{stderrPath}
	poll = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTailers(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	waitOrFail(t, "tailers did not stop after context cancellation")
}

// waitOrFail waits for wg with a timeout, failing the test with msg if it does not finish.
func waitOrFail(t *testing.T, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}

func TestUsage(t *testing.T) {
	if fatalCase("usage") {
		flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
		registerFlags()
		usage()
		return
	}

	// usage writes with the builtin println, which bypasses os.Stderr, so its text is checked in a child process.
	usage()

	exitCode, stderr := runFatalCase(t, "TestUsage", "usage")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stderr, "Usage: dockerize [options] [command]")
	assert.Contains(t, stderr, "-wait-http-header")
	assert.Contains(t, stderr, "For more information, see https://github.com/jwilder/dockerize")
}

func TestMainPrintsVersion(t *testing.T) {
	resetGlobals(t)
	oldArgs, oldCommandLine, oldUsage, oldBuildVersion := os.Args, flag.CommandLine, flag.Usage, buildVersion
	t.Cleanup(func() {
		os.Args, flag.CommandLine, flag.Usage, buildVersion = oldArgs, oldCommandLine, oldUsage, oldBuildVersion
	})
	flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
	os.Args = []string{"dockerize", "-version"}
	buildVersion = "v1.2.3"

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	main()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3\n", buf.String())
}

func TestMainFatalCases(t *testing.T) {
	if fatalCase("no-args") {
		flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
		os.Args = []string{"dockerize"}
		main()
	}
	if fatalCase("bad-delims") {
		flag.CommandLine = flag.NewFlagSet("dockerize", flag.ContinueOnError)
		os.Args = []string{"dockerize", "-delims", "a:b:c"}
		main()
	}

	t.Run("no args prints usage and exits 1", func(t *testing.T) {
		exitCode, stderr := runFatalCase(t, "TestMainFatalCases", "no-args")
		assert.Equal(t, 1, exitCode)
		assert.Contains(t, stderr, "Usage: dockerize [options] [command]")
	})

	t.Run("bad delimiters exit 1 with error", func(t *testing.T) {
		exitCode, stderr := runFatalCase(t, "TestMainFatalCases", "bad-delims")
		assert.Equal(t, 1, exitCode)
		assert.Contains(t, stderr, `bad delimiters argument: a:b:c. expected "left:right"`)
	})
}
