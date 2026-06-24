package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// saveTemplateGlobals saves the current values of delims and noOverwriteFlag,
// sets them to the given values, and returns a cleanup function that restores
// the originals. Intended for use with t.Cleanup.
func saveTemplateGlobals(t *testing.T, d []string, noOverwrite bool) {
	t.Helper()
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	delims = d
	noOverwriteFlag = noOverwrite
	t.Cleanup(func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	})
}

func TestGenerateFileRendersTemplateFunctions(t *testing.T) {
	saveTemplateGlobals(t, nil, false)

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
	saveTemplateGlobals(t, []string{"[[", "]]"}, false)

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
	saveTemplateGlobals(t, nil, true)

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
	saveTemplateGlobals(t, nil, false)

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
	missingPath := filepath.Join("/proc", "1", "fd", "does-not-exist", "child")

	_, err := exists(missingPath)
	if err == nil {
		t.Fatal("exists returned nil error, want error")
	}
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("exists error %q does not include path %q", err.Error(), missingPath)
	}
}

func TestJSONQueryErrorsIncludeContext(t *testing.T) {
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
	saveTemplateGlobals(t, nil, false)

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
