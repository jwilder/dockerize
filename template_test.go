package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateFileRendersTemplateFunctions(t *testing.T) {
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

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
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

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
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

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

func TestGenerateDirRendersAllTemplates(t *testing.T) {
	oldDelims := delims
	oldNoOverwrite := noOverwriteFlag
	defer func() {
		delims = oldDelims
		noOverwriteFlag = oldNoOverwrite
	}()

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
