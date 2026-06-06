package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateFile_ToFile(t *testing.T) {
	// Create a temporary template file
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "test.tmpl")
	destPath := filepath.Join(tmpDir, "output.txt")

	err := os.WriteFile(templatePath, []byte("Hello {{ .Env.USER | default \"world\" }}"), 0644)
	assert.NoError(t, err)

	// Reset globals that generateFile depends on
	delims = nil
	noOverwriteFlag = false

	result := generateFile(templatePath, destPath)
	assert.True(t, result)

	// Verify the output file was created and has content
	content, err := os.ReadFile(destPath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "Hello")
}

func TestGenerateFile_ToStdout(t *testing.T) {
	// Create a temporary template file
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "test.tmpl")

	err := os.WriteFile(templatePath, []byte("Hello world"), 0644)
	assert.NoError(t, err)

	// Reset globals
	delims = nil
	noOverwriteFlag = false

	// destPath="" means write to stdout
	result := generateFile(templatePath, "")
	assert.True(t, result)
}

func TestGenerateFile_NoOverwrite(t *testing.T) {
	// Create a temporary template file and pre-existing dest file
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "test.tmpl")
	destPath := filepath.Join(tmpDir, "output.txt")

	err := os.WriteFile(templatePath, []byte("New content"), 0644)
	assert.NoError(t, err)

	err = os.WriteFile(destPath, []byte("Original content"), 0644)
	assert.NoError(t, err)

	// Set no-overwrite flag
	delims = nil
	noOverwriteFlag = true

	result := generateFile(templatePath, destPath)
	assert.False(t, result)

	// Verify original content was preserved
	content, err := os.ReadFile(destPath)
	assert.NoError(t, err)
	assert.Equal(t, "Original content", string(content))

	// Reset for other tests
	noOverwriteFlag = false
}

func TestGenerateFile_WithCustomDelims(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "test.tmpl")
	destPath := filepath.Join(tmpDir, "output.txt")

	err := os.WriteFile(templatePath, []byte("Hello <% .Env.USER | default \"world\" %>"), 0644)
	assert.NoError(t, err)

	delims = []string{"<%", "%>"}
	noOverwriteFlag = false

	result := generateFile(templatePath, destPath)
	assert.True(t, result)

	content, err := os.ReadFile(destPath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "Hello")

	// Reset
	delims = nil
}

func TestJsonQuery(t *testing.T) {
	jsonStr := `{"name": "test", "value": 42}`

	result, err := jsonQuery(jsonStr, "name")
	assert.NoError(t, err)
	assert.Equal(t, "test", result)

	_, err = jsonQuery("invalid json", "key")
	assert.Error(t, err)

	_, err = jsonQuery(jsonStr, ".nonexistent.deep.path")
	assert.Error(t, err)
}
