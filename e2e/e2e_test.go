package e2e_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var dockerizeBin string

func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests
	tmpDir, err := os.MkdirTemp("", "dockerize-e2e-*")
	if err != nil {
		log.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dockerizeBin = filepath.Join(tmpDir, "dockerize")

	repoRoot, err := filepath.Abs(filepath.Join(".", ".."))
	if err != nil {
		log.Fatalf("failed to resolve repo root: %v", err)
	}

	buildCmd := exec.Command("go", "build", "-o", dockerizeBin, ".")
	buildCmd.Dir = repoRoot
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("failed to build dockerize: %v\n%s", err, out)
	}

	os.Exit(m.Run())
}

// --- CLI behavior ---

func TestVersion(t *testing.T) {
	cmd := exec.Command(dockerizeBin, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v\n%s", err, out)
	}
	// Built without ldflags, so version is empty — just verify no crash
}

func TestNoArgsShowsUsage(t *testing.T) {
	cmd := exec.Command(dockerizeBin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit")
	}

	if !strings.Contains(string(out), "Usage: dockerize") {
		t.Fatalf("expected usage text, got: %s", string(out))
	}
}

func TestExitCodePassthrough(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
	}{
		{"exit-1", 1},
		{"exit-2", 2},
		{"exit-42", 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(dockerizeBin, "sh", "-c", fmt.Sprintf("exit %d", tt.exitCode))
			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected non-zero exit")
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if exitErr.ExitCode() != tt.exitCode {
					t.Fatalf("expected exit code %d, got %d", tt.exitCode, exitErr.ExitCode())
				}
			} else {
				t.Fatalf("expected ExitError, got %T: %v", err, err)
			}
		})
	}
}

// --- Template rendering ---

func TestTemplateBasic(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "test.tmpl")
	destFile := filepath.Join(dir, "output.conf")

	os.WriteFile(tmplFile, []byte(`server {{ .Env.TEST_HOST }}:{{ .Env.TEST_PORT }}`), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = append(os.Environ(), "TEST_HOST=localhost", "TEST_PORT=8080")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := "server localhost:8080"
	if string(got) != expected {
		t.Fatalf("expected %q, got %q", expected, string(got))
	}
}

func TestTemplateToStdout(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "hello.tmpl")
	os.WriteFile(tmplFile, []byte(`hello {{ .Env.NAME }}`), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile)
	cmd.Env = append(os.Environ(), "NAME=world")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "hello world") {
		t.Fatalf("expected 'hello world' in output, got %q", string(out))
	}
}

func TestTemplateCustomDelims(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "custom.tmpl")
	destFile := filepath.Join(dir, "output.txt")

	os.WriteFile(tmplFile, []byte(`value=<% .Env.MY_VAR %>`), 0644)

	cmd := exec.Command(dockerizeBin, "-delims", "<%:%>", "-template", tmplFile+":"+destFile)
	cmd.Env = append(os.Environ(), "MY_VAR=custom_delims_work")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if string(got) != "value=custom_delims_work" {
		t.Fatalf("expected 'value=custom_delims_work', got %q", string(got))
	}
}

func TestTemplateNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "tmpl.tmpl")
	destFile := filepath.Join(dir, "dest.txt")

	os.WriteFile(tmplFile, []byte(`new content`), 0644)
	os.WriteFile(destFile, []byte(`original content`), 0644)

	cmd := exec.Command(dockerizeBin, "-no-overwrite", "-template", tmplFile+":"+destFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if string(got) != "original content" {
		t.Fatalf("expected file to remain 'original content', got %q", string(got))
	}
}

func TestTemplateDirectory(t *testing.T) {
	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "templates")
	destDir := filepath.Join(dir, "output")
	os.MkdirAll(tmplDir, 0755)
	os.MkdirAll(destDir, 0755)

	os.WriteFile(filepath.Join(tmplDir, "a.conf"), []byte(`a={{ .Env.A }}`), 0644)
	os.WriteFile(filepath.Join(tmplDir, "b.conf"), []byte(`b={{ .Env.B }}`), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplDir+":"+destDir)
	cmd.Env = append(os.Environ(), "A=alpha", "B=beta")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	gotA, _ := os.ReadFile(filepath.Join(destDir, "a.conf"))
	gotB, _ := os.ReadFile(filepath.Join(destDir, "b.conf"))
	if string(gotA) != "a=alpha" {
		t.Fatalf("a.conf: expected 'a=alpha', got %q", string(gotA))
	}
	if string(gotB) != "b=beta" {
		t.Fatalf("b.conf: expected 'b=beta', got %q", string(gotB))
	}
}

func TestTemplateSprigFunctions(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "sprig.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `{{ .Env.GREETING | upper }}_{{ "  padded  " | trimAll " " }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = append(os.Environ(), "GREETING=hello")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if string(got) != "HELLO_padded" {
		t.Fatalf("expected 'HELLO_padded', got %q", string(got))
	}
}

// --- File tailing ---

func TestStdoutTail(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	os.WriteFile(logFile, []byte("line1\nline2\n"), 0644)

	cmd := exec.Command(dockerizeBin, "-stdout", logFile, "-poll", "sh", "-c", "sleep 0.5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "line1") || !strings.Contains(output, "line2") {
		t.Fatalf("expected tailed lines in output, got: %s", output)
	}
}

func TestStderrTail(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "error.log")
	os.WriteFile(logFile, []byte("err1\nerr2\n"), 0644)

	cmd := exec.Command(dockerizeBin, "-stderr", logFile, "-poll", "sh", "-c", "sleep 0.5")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("dockerize failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stderr.String()
	if !strings.Contains(output, "err1") || !strings.Contains(output, "err2") {
		t.Fatalf("expected tailed lines in stderr, got: %s", output)
	}
}

// --- Wait for dependencies ---

func TestWaitTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	cmd := exec.Command(dockerizeBin, "-wait", "tcp://"+addr, "-timeout", "5s", "echo", "ready")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "ready") {
		t.Fatalf("expected 'ready' in output, got: %s", string(out))
	}
}

func TestWaitTCPTimeout(t *testing.T) {
	// Use a port that nothing is listening on
	cmd := exec.Command(dockerizeBin, "-wait", "tcp://127.0.0.1:19999", "-timeout", "1s", "echo", "should-not-run")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, but got success\n%s", out)
	}

	if !strings.Contains(string(out), "Timeout") {
		t.Fatalf("expected timeout message, got: %s", string(out))
	}
}

func TestWaitHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cmd := exec.Command(dockerizeBin, "-wait", srv.URL, "-timeout", "5s", "echo", "http-ready")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "http-ready") {
		t.Fatalf("expected 'http-ready' in output, got: %s", string(out))
	}
}

func TestWaitHTTPDelayed(t *testing.T) {
	var ready atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer srv.Close()

	go func() {
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	cmd := exec.Command(dockerizeBin, "-wait", srv.URL, "-timeout", "10s",
		"-wait-retry-interval", "500ms", "echo", "eventually-ready")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "eventually-ready") {
		t.Fatalf("expected 'eventually-ready' in output, got: %s", string(out))
	}
}

func TestWaitFile(t *testing.T) {
	dir := t.TempDir()
	waitFile := filepath.Join(dir, "ready.flag")

	go func() {
		time.Sleep(1 * time.Second)
		os.WriteFile(waitFile, []byte("ok"), 0644)
	}()

	cmd := exec.Command(dockerizeBin, "-wait", "file://"+waitFile, "-timeout", "5s", "echo", "file-ready")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "file-ready") {
		t.Fatalf("expected 'file-ready' in output, got: %s", string(out))
	}
}

func TestWaitMultiple(t *testing.T) {
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln1.Close()
	defer ln2.Close()

	cmd := exec.Command(dockerizeBin,
		"-wait", "tcp://"+ln1.Addr().String(),
		"-wait", "tcp://"+ln2.Addr().String(),
		"-timeout", "5s",
		"echo", "both-ready")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "both-ready") {
		t.Fatalf("expected 'both-ready' in output, got: %s", string(out))
	}
}

func TestWaitHTTPHeader(t *testing.T) {
	receivedHeader := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerValue := r.Header.Get("X-Custom-Auth")
		receivedHeader <- headerValue
		if headerValue == "secret123" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()

	cmd := exec.Command(dockerizeBin,
		"-wait", srv.URL,
		"-wait-http-header", "X-Custom-Auth: secret123",
		"-timeout", "5s",
		"echo", "authed")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	select {
	case got := <-receivedHeader:
		if got != "secret123" {
			t.Fatalf("expected header value %q, got %q", "secret123", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP request")
	}

	if !strings.Contains(string(out), "authed") {
		t.Fatalf("expected 'authed' in output, got: %s", string(out))
	}
}

func TestWaitHTTPHeaderValueWithAdditionalColons(t *testing.T) {
	const expectedValue = "Bearer part1:part2:part3"
	receivedHeader := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerValue := r.Header.Get("X-Custom-Auth")
		select {
		case receivedHeader <- headerValue:
		default:
		}
		if headerValue == expectedValue {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()

	cmd := exec.Command(dockerizeBin,
		"-wait", srv.URL,
		"-wait-http-header", "X-Custom-Auth: "+expectedValue,
		"-timeout", "5s",
		"echo", "authed")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	select {
	case got := <-receivedHeader:
		if got != expectedValue {
			t.Fatalf("expected header value %q, got %q", expectedValue, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP request")
	}

	if !strings.Contains(string(out), "authed") {
		t.Fatalf("expected 'authed' in output, got: %s", string(out))
	}
}

// --- .Env context interface ---

func TestTemplateEnvContext(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "env.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `host={{ .Env.DB_HOST }} port={{ .Env.DB_PORT }} pass={{ .Env.DB_PASS }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = append(os.Environ(), "DB_HOST=pg.local", "DB_PORT=5432", "DB_PASS=p@ss=w0rd!")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := `host=pg.local port=5432 pass=p@ss=w0rd!`
	if string(got) != expected {
		t.Fatalf("expected %q, got %q", expected, string(got))
	}
}

func TestTemplateEnvRange(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "range.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	// Range over .Env filtering by prefix
	tmpl := `{{ range $k, $v := .Env }}{{ if hasPrefix "MYAPP_" $k }}{{ $k }}={{ $v }}` + "\n" + `{{ end }}{{ end }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"MYAPP_HOST=web1", "MYAPP_PORT=9090", "OTHER_VAR=ignore"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if !strings.Contains(string(got), "MYAPP_HOST=web1") || !strings.Contains(string(got), "MYAPP_PORT=9090") {
		t.Fatalf("expected MYAPP_ vars, got %q", string(got))
	}
	if strings.Contains(string(got), "OTHER_VAR") {
		t.Fatalf("unexpected OTHER_VAR in output: %q", string(got))
	}
}

func TestTemplateEnvMissing(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "missing.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `before[{{ .Env.DEFINITELY_NOT_SET }}]after`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	// Deliberately don't set DEFINITELY_NOT_SET
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if string(got) != "before[<no value>]after" {
		t.Fatalf("expected 'before[<no value>]after', got %q", string(got))
	}
}

// --- Template functions ---

func TestTemplateFuncDefault(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "default.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `port={{ default .Env.PORT "3000" }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	// Test with PORT unset — should use default
	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"HOME=/tmp"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(destFile)
	if string(got) != "port=3000" {
		t.Fatalf("expected 'port=3000', got %q", string(got))
	}

	// Test with PORT set — should use actual value
	cmd2 := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd2.Env = []string{"HOME=/tmp", "PORT=8080"}
	out, err = cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}
	got, _ = os.ReadFile(destFile)
	if string(got) != "port=8080" {
		t.Fatalf("expected 'port=8080', got %q", string(got))
	}
}

func TestTemplateFuncContains(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "contains.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `{{ if contains .Env "SECRET" }}has-secret{{ else }}no-secret{{ end }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	// With SECRET set
	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"SECRET=abc123"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(destFile)
	if string(got) != "has-secret" {
		t.Fatalf("expected 'has-secret', got %q", string(got))
	}

	// Without SECRET
	cmd2 := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd2.Env = []string{"HOME=/tmp"}
	out, err = cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}
	got, _ = os.ReadFile(destFile)
	if string(got) != "no-secret" {
		t.Fatalf("expected 'no-secret', got %q", string(got))
	}
}

func TestTemplateFuncExists(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "present.conf")
	os.WriteFile(existingFile, []byte("x"), 0644)

	tmplFile := filepath.Join(dir, "exists.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	// exists returns (bool, error) — use with index or assign
	tmpl := fmt.Sprintf(`{{ $e := exists "%s" }}{{ $m := exists "%s/absent.conf" }}{{ $e }}-{{ $m }}`,
		existingFile, dir)
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	if string(got) != "true-false" {
		t.Fatalf("expected 'true-false', got %q", string(got))
	}
}

func TestTemplateFuncStringAndMath(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "funcs.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `{{ $parts := split .Env.CSV "," }}first={{ index $parts 0 }}
replaced={{ replace .Env.GREETING "hello" "hi" 1 }}
lower={{ lower .Env.UPPER }}
upper={{ upper .Env.LOWER }}
sum={{ add (atoi .Env.A) (atoi .Env.B) }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"CSV=one,two,three", "GREETING=hello world", "UPPER=LOUD", "LOWER=quiet", "A=10", "B=32"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := "first=one\nreplaced=hi world\nlower=loud\nupper=QUIET\nsum=42"
	if string(got) != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, string(got))
	}
}

func TestTemplateFuncParseUrl(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "url.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `{{ $u := parseUrl .Env.DATABASE_URL }}host={{ $u.Host }} scheme={{ $u.Scheme }} path={{ $u.Path }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"DATABASE_URL=postgres://db.example.com:5432/mydb"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := "host=db.example.com:5432 scheme=postgres path=/mydb"
	if string(got) != expected {
		t.Fatalf("expected %q, got %q", expected, string(got))
	}
}

func TestTemplateFuncIsTrue(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "istrue.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `{{ if isTrue .Env.ENABLED }}on{{ else }}off{{ end }}-{{ if isTrue .Env.DISABLED }}on{{ else }}off{{ end }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	tests := []struct {
		enabled  string
		disabled string
		expected string
	}{
		{"true", "false", "on-off"},
		{"1", "0", "on-off"},
		{"yes", "no", "on-off"},
		{"on", "off", "on-off"},
		{"TRUE", "FALSE", "on-off"},
	}

	for _, tt := range tests {
		t.Run(tt.enabled+"/"+tt.disabled, func(t *testing.T) {
			cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
			cmd.Env = []string{"ENABLED=" + tt.enabled, "DISABLED=" + tt.disabled}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dockerize failed (ENABLED=%s): %v\n%s", tt.enabled, err, out)
			}
			got, _ := os.ReadFile(destFile)
			if string(got) != tt.expected {
				t.Fatalf("ENABLED=%s DISABLED=%s: expected %q, got %q", tt.enabled, tt.disabled, tt.expected, string(got))
			}
		})
	}
}

func TestTemplateFuncJsonQuery(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "json.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	tmpl := `name={{ jsonQuery .Env.CONFIG "name" }} version={{ jsonQuery .Env.CONFIG "version" }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	jsonVal := `{"name":"myapp","version":"1.2.3"}`
	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"CONFIG=" + jsonVal}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := "name=myapp version=1.2.3"
	if string(got) != expected {
		t.Fatalf("expected %q, got %q", expected, string(got))
	}
}

func TestTemplateFuncLoop(t *testing.T) {
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "loop.tmpl")
	destFile := filepath.Join(dir, "out.txt")

	// Test all three forms: loop(stop), loop(start, stop), loop(start, stop, step)
	tmpl := `a={{ range $i := loop 3 }}{{ $i }}{{ end }}
b={{ range $i := loop 2 5 }}{{ $i }}{{ end }}
c={{ range $i := loop 0 10 3 }}{{ $i }}{{ end }}`
	os.WriteFile(tmplFile, []byte(tmpl), 0644)

	cmd := exec.Command(dockerizeBin, "-template", tmplFile+":"+destFile)
	cmd.Env = []string{"HOME=/tmp"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerize failed: %v\n%s", err, out)
	}

	got, _ := os.ReadFile(destFile)
	expected := "a=012\nb=234\nc=0369"
	if string(got) != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, string(got))
	}
}
