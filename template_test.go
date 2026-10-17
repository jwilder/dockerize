package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"text/template"
)

func TestGenerateFileRendersTemplateFunctions(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "config.tmpl")
	destPath := filepath.Join(tempDir, "config.out")

	templateContent := `{{ default nil "fallback" }}|{{ add 2 3 }}|{{ lower "MiXeD" }}|{{ upper "MiXeD" }}|{{ isTrue "yes" }}|{{ jsonQuery "{\"name\":\"dockerize\"}" "name" }}|{{ range $i := loop 1 4 }}{{ $i }}{{ end }}`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if ok := generateFile(templatePath, destPath); !ok {
		t.Fatalf("generateFile returned false")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}

	want := "fallback|5|mixed|MIXED|true|dockerize|123"
	if string(got) != want {
		t.Fatalf("rendered output mismatch: got %q want %q", string(got), want)
	}
}

func TestGenerateFileUsesCustomDelimiters(t *testing.T) {
	resetGlobals(t)
	delims = []string{"[[", "]]"}
	noOverwriteFlag = false

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "custom.tmpl")
	destPath := filepath.Join(tempDir, "custom.out")

	if err := os.WriteFile(templatePath, []byte(`[[ add 7 8 ]]`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if ok := generateFile(templatePath, destPath); !ok {
		t.Fatalf("generateFile returned false")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}

	if string(got) != "15" {
		t.Fatalf("rendered output mismatch: got %q want %q", string(got), "15")
	}
}

func TestGenerateFileNoOverwrite(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = true

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "skip.tmpl")
	destPath := filepath.Join(tempDir, "skip.out")

	if err := os.WriteFile(templatePath, []byte(`new-content`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("existing-content"), 0o644); err != nil {
		t.Fatalf("write existing dest: %v", err)
	}

	if ok := generateFile(templatePath, destPath); ok {
		t.Fatalf("generateFile returned true, want false when no-overwrite is enabled")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read destination file: %v", err)
	}

	if string(got) != "existing-content" {
		t.Fatalf("destination file was overwritten: got %q", string(got))
	}
}

func TestGenerateFileDefaultDoesNotPanicOnNonStringValue(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "default-non-string.tmpl")
	destPath := filepath.Join(tempDir, "default-non-string.out")

	if err := os.WriteFile(templatePath, []byte(`{{ default (add 1 2) "fallback" }}`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("generateFile panicked: %v", r)
		}
	}()

	generateFile(templatePath, destPath)
}

func TestExistsErrorIncludesFilePath(t *testing.T) {
	t.Parallel()

	// A path through a regular file fails with ENOTDIR rather than ENOENT,
	// regardless of the user running the test.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(file, "child")

	_, err := exists(missingPath)
	if err == nil {
		t.Fatal("exists returned nil error, want error")
	}
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("exists error %q does not include path %q", err.Error(), missingPath)
	}
}

func TestJSONQueryErrorsIncludeContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		jsonObj   string
		query     string
		wantParts []string
	}{
		{
			name:      "invalid json includes json object",
			jsonObj:   `{`,
			query:     "name",
			wantParts: []string{"{"},
		},
		{
			name:      "invalid query includes query and json object",
			jsonObj:   `{"name":"dockerize"}`,
			query:     "[",
			wantParts: []string{"[", `{"name":"dockerize"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := jsonQuery(tt.jsonObj, tt.query)
			if err == nil {
				t.Fatal("jsonQuery returned nil error, want error")
			}
			for _, wantPart := range tt.wantParts {
				if !strings.Contains(err.Error(), wantPart) {
					t.Fatalf("jsonQuery error %q does not include context %q", err.Error(), wantPart)
				}
			}
		})
	}
}

func TestLoopChannelConsumesAllProducedValuesInOrder(t *testing.T) {
	t.Parallel()

	ch, err := loop(1, 10, 3)
	if err != nil {
		t.Fatalf("loop returned error: %v", err)
	}

	var got []int
	for v := range ch {
		got = append(got, v)
	}

	want := []int{1, 4, 7}
	if len(got) != len(want) {
		t.Fatalf("consumed %d values, want %d: got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value %d mismatch: got %d want %d (full sequence got %v want %v)", i, got[i], want[i], got, want)
		}
	}
}

func TestGenerateDirRendersAllTemplates(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	templateDir := filepath.Join(tempDir, "templates")
	destDir := filepath.Join(tempDir, "rendered")

	if err := os.Mkdir(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatalf("mkdir dest dir: %v", err)
	}

	files := map[string]string{
		"one.txt": `{{ add 1 1 }}`,
		"two.txt": `{{ upper "two" }}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(templateDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write template %s: %v", name, err)
		}
	}

	if ok := generateDir(templateDir, destDir); !ok {
		t.Fatalf("generateDir returned false")
	}

	checks := map[string]string{
		"one.txt": "2",
		"two.txt": "TWO",
	}
	for name, want := range checks {
		got, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil {
			t.Fatalf("read rendered file %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("rendered output mismatch for %s: got %q want %q", name, string(got), want)
		}
	}
}

func TestGenerateFileWritesFullContentAndKeepsMode(t *testing.T) {
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "big.tmpl")
	destPath := filepath.Join(tempDir, "big.out")

	templateContent := strings.Repeat("line {{ add 1 2 }} {{ upper \"abc\" }}\n", 5000)
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(destPath, []byte("stale"), 0o640); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}
	if err := os.Chmod(destPath, 0o640); err != nil {
		t.Fatalf("chmod destination: %v", err)
	}

	if ok := generateFile(templatePath, destPath); !ok {
		t.Fatalf("generateFile returned false")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	want := strings.Repeat("line 3 ABC\n", 5000)
	if string(got) != want {
		t.Fatalf("rendered output mismatch: got %d bytes want %d bytes", len(got), len(want))
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat rendered file: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o640 {
		t.Fatalf("mode mismatch: got %o want %o", mode, 0o640)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if int(st.Uid) != os.Getuid() || int(st.Gid) != os.Getgid() {
			t.Fatalf("owner mismatch: got %d:%d want %d:%d", st.Uid, st.Gid, os.Getuid(), os.Getgid())
		}
	}
}

func TestLoopRejectsNonPositiveStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []int
		wantE string
	}{
		{name: "zero step", args: []int{0, 5, 0}, wantE: "loop: step must be positive, got 0"},
		{name: "negative step", args: []int{0, 5, -1}, wantE: "loop: step must be positive, got -1"},
		{name: "negative step with start above stop", args: []int{5, 0, -1}, wantE: "loop: step must be positive, got -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, err := loop(tt.args...)
			if err == nil {
				t.Fatalf("loop(%v) returned no error (channel %v), want error containing %q", tt.args, ch, tt.wantE)
			}
			if err.Error() != tt.wantE {
				t.Fatalf("loop(%v) error = %q, want %q", tt.args, err.Error(), tt.wantE)
			}
		})
	}
}

// limitedWriter fails once more than limit bytes are written, so a template
// that would otherwise stream forever stops with an error.
type limitedWriter struct {
	limit   int
	written int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.written += len(p)
	if w.written > w.limit {
		return 0, errors.New("output limit reached")
	}
	return len(p), nil
}

func TestTemplateRangeOverNonPositiveStepFails(t *testing.T) {
	t.Parallel()

	tmpl, err := template.New("loop").Funcs(templateFuncMap()).Parse("{{range loop 0 5 0}}x{{end}}")
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	err = tmpl.Execute(&limitedWriter{limit: 1024}, nil)
	const wantE = "loop: step must be positive, got 0"
	if err == nil || !strings.Contains(err.Error(), wantE) {
		t.Fatalf("Execute error = %v, want error containing %q", err, wantE)
	}
}

func TestTemplateFuncMapDockerizeOverridesSprig(t *testing.T) {
	merged := templateFuncMap()

	dockerizeFuncs := map[string]any{
		"contains":  contains,
		"exists":    exists,
		"split":     strings.Split,
		"replace":   strings.Replace,
		"default":   defaultValue,
		"parseUrl":  parseUrl,
		"atoi":      strconv.Atoi,
		"add":       add,
		"isTrue":    isTrue,
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"jsonQuery": jsonQuery,
		"loop":      loop,
	}
	for name, fn := range dockerizeFuncs {
		got, ok := merged[name]
		if !ok {
			t.Fatalf("%s missing from merged func map", name)
		}
		if reflect.ValueOf(got).Pointer() != reflect.ValueOf(fn).Pointer() {
			t.Errorf("%s: merged map does not hold dockerize's function", name)
		}
	}
}

func TestGenerateFileRendersReadmeExamples(t *testing.T) {
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

	delims = nil
	noOverwriteFlag = false

	t.Setenv("PATH", "/usr/bin:/bin:/usr/local/bin")
	t.Setenv("DOCKERIZE_README_SET", "1")
	if err := os.Unsetenv("DOCKERIZE_README_UNSET"); err != nil {
		t.Fatalf("unset env: %v", err)
	}

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "readme.tmpl")
	destPath := filepath.Join(tempDir, "readme.out")

	templateContent := `{{ replace .Env.PATH ":" ";" -1 }}|{{ if contains .Env "DOCKERIZE_README_SET" }}set{{ else }}unset{{ end }}|{{ if contains .Env "DOCKERIZE_README_UNSET" }}set{{ else }}unset{{ end }}`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if ok := generateFile(templatePath, destPath); !ok {
		t.Fatalf("generateFile returned false")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}

	want := "/usr/bin;/bin;/usr/local/bin|set|unset"
	if string(got) != want {
		t.Fatalf("rendered output mismatch: got %q want %q", string(got), want)
	}
}

func TestGenerateFileRendersJSONQueryReadmeExample(t *testing.T) {
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

	delims = nil
	noOverwriteFlag = false

	t.Setenv("SERVICES", `{
  "services": [
    {
      "name": "service1",
      "port": 8000
    },{
      "name": "service2",
      "port": 9000
    }
  ]
}`)

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "jsonquery.tmpl")
	destPath := filepath.Join(tempDir, "jsonquery.out")

	templateContent := `{{ jsonQuery .Env.SERVICES "services\\.[1]\\.port" }}`
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if ok := generateFile(templatePath, destPath); !ok {
		t.Fatalf("generateFile returned false")
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}

	want := "9000"
	if string(got) != want {
		t.Fatalf("rendered output mismatch: got %q want %q", string(got), want)
	}
}
