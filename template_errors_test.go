package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTemplateFile writes content to dir/name and returns the path.
func writeTemplateFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}

// templateFatalCase is one log.Fatalf path in template processing. run sets up its fixtures and makes the
// fatal call; it runs only in the re-executed child, which must exit 1 with wantErr on stderr.
type templateFatalCase struct {
	name     string
	skipRoot bool
	run      func(t *testing.T)
	wantErr  string
}

var templateFatalCases = []templateFatalCase{
	{
		name: "missing-src",
		run: func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "missing.tmpl")
			processTemplates([]string{src + ":" + filepath.Join(t.TempDir(), "out")})
		},
		wantErr: "unable to stat",
	},
	{
		name: "dir-dest-missing",
		run: func(t *testing.T) {
			srcDir := t.TempDir()
			writeTemplateFile(t, srcDir, "a.tmpl", "a")
			processTemplates([]string{srcDir + ":" + filepath.Join(t.TempDir(), "missing")})
		},
		wantErr: "unable to stat",
	},
	{
		name: "dir-dest-is-file",
		run: func(t *testing.T) {
			srcDir := t.TempDir()
			writeTemplateFile(t, srcDir, "a.tmpl", "a")
			destFile := writeTemplateFile(t, t.TempDir(), "dest", "")
			processTemplates([]string{srcDir + ":" + destFile})
		},
		wantErr: "if template is a directory, dest must also be a directory",
	},
	{
		name:     "dir-unreadable",
		skipRoot: true,
		run: func(t *testing.T) {
			srcDir := filepath.Join(t.TempDir(), "tmpl")
			if err := os.Mkdir(srcDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			writeTemplateFile(t, srcDir, "a.tmpl", "a")
			if err := os.Chmod(srcDir, 0); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			processTemplates([]string{srcDir + ":" + t.TempDir()})
		},
		wantErr: "bad directory",
	},
	{
		name: "template-syntax",
		run: func(t *testing.T) {
			src := writeTemplateFile(t, t.TempDir(), "bad.tmpl", "{{ if }}")
			processTemplates([]string{src + ":" + filepath.Join(t.TempDir(), "out")})
		},
		wantErr: "unable to parse template",
	},
	{
		name: "template-exec",
		run: func(t *testing.T) {
			// add takes ints, so passing a string fails at execution.
			src := writeTemplateFile(t, t.TempDir(), "bad.tmpl", `{{ add "a" 1 }}`)
			processTemplates([]string{src + ":" + filepath.Join(t.TempDir(), "out")})
		},
		wantErr: "template error",
	},
}

func TestTemplateFatalPaths(t *testing.T) {
	for _, tc := range templateFatalCases {
		if fatalCase(tc.name) {
			tc.run(t)
			return
		}
	}

	for _, tc := range templateFatalCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipRoot && os.Getuid() == 0 {
				t.Skip("unreadable directory is readable to root")
			}

			exitCode, stderr := runFatalCase(t, "TestTemplateFatalPaths", tc.name)
			assert.Equal(t, 1, exitCode)
			assert.Contains(t, stderr, tc.wantErr)
		})
	}
}

func TestParseUrlBadURLReturnsError(t *testing.T) {
	t.Parallel()

	got, err := parseUrl("http://a b/%zz")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorContains(t, err, `unable to parse url "http://a b/%zz"`)
}

func TestTemplateParseUrlBadURLIsExecutionError(t *testing.T) {
	t.Parallel()

	tmpl := template.Must(template.New("url").Funcs(templateFuncMap()).Parse(`{{ parseUrl "http://a b/%zz" }}`))
	err := tmpl.Execute(io.Discard, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "unable to parse url")
}

func TestTemplateParseUrlGoodURLRendersFields(t *testing.T) {
	t.Parallel()

	tmpl := template.Must(template.New("url").Funcs(templateFuncMap()).Parse(
		`{{ $u := parseUrl .X }}{{ $u.Scheme }} {{ $u.Host }} {{ $u.Path }} {{ (parseUrl .X).Host }}`))
	var out bytes.Buffer
	err := tmpl.Execute(&out, map[string]string{"X": "postgres://db.example.com:5432/mydb"})
	require.NoError(t, err)
	assert.Equal(t, "postgres db.example.com:5432 /mydb db.example.com:5432", out.String())
}

func TestProcessTemplatesSplitsDestOnFirstColon(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	src := writeTemplateFile(t, tempDir, "config.tmpl", "{{ lower \"OK\" }}")
	dest := filepath.Join(tempDir, "a:b.out")

	processTemplates([]string{src + ":" + dest})

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(got))
}
