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

// fixtureDirEnv passes a parent-built fixture directory to a re-executed child process.
const fixtureDirEnv = "DOCKERIZE_TEST_FIXTURE_DIR"

// A render that fails part way must leave the existing destination untouched and no temp files behind.
func TestProcessTemplatesFailedRenderKeepsDestination(t *testing.T) {
	const caseName = "failed-render-keeps-dest"
	if fatalCase(caseName) {
		dir := os.Getenv(fixtureDirEnv)
		processTemplates([]string{filepath.Join(dir, "bad.tmpl") + ":" + filepath.Join(dir, "out.conf")})
		return
	}

	dir := t.TempDir()
	const original = "original contents\n"
	writeTemplateFile(t, dir, "out.conf", original)
	// Output is written before the add call fails, so a render that streams into the file would leave a partial file.
	writeTemplateFile(t, dir, "bad.tmpl", `partial output {{ add "a" 1 }}`)
	t.Setenv(fixtureDirEnv, dir)

	exitCode, stderr := runFatalCase(t, "TestProcessTemplatesFailedRenderKeepsDestination", caseName)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "template error")

	got, err := os.ReadFile(filepath.Join(dir, "out.conf"))
	require.NoError(t, err)
	assert.Equal(t, original, string(got))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"bad.tmpl", "out.conf"}, names)
}

// A template dir that contains a subdirectory renders its files and skips the subdirectory.
func TestProcessTemplatesSkipsSubdirectory(t *testing.T) {
	const caseName = "skips-subdirectory"
	if fatalCase(caseName) {
		dir := os.Getenv(fixtureDirEnv)
		processTemplates([]string{filepath.Join(dir, "tmpl") + ":" + filepath.Join(dir, "out")})
		return
	}

	dir := t.TempDir()
	tmplDir := filepath.Join(dir, "tmpl")
	require.NoError(t, os.MkdirAll(filepath.Join(tmplDir, "sub"), 0o755))
	writeTemplateFile(t, tmplDir, "a.conf", `hello {{ lower "WORLD" }}`)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "out"), 0o755))
	t.Setenv(fixtureDirEnv, dir)

	exitCode, stderr := runFatalCase(t, "TestProcessTemplatesSkipsSubdirectory", caseName)
	assert.Equal(t, 0, exitCode, stderr)

	got, err := os.ReadFile(filepath.Join(dir, "out", "a.conf"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(got))
}

// Rewriting an existing destination keeps its file mode.
func TestProcessTemplatesKeepsExistingMode(t *testing.T) {
	resetGlobals(t)
	delims = nil
	noOverwriteFlag = false

	tempDir := t.TempDir()
	src := writeTemplateFile(t, tempDir, "config.tmpl", "new")
	dest := writeTemplateFile(t, tempDir, "out.conf", "old")
	require.NoError(t, os.Chmod(dest, 0o600))

	processTemplates([]string{src + ":" + dest})

	fi, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
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
