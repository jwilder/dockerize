package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/jwilder/gojq"
)

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("unable to stat %s: %w", path, err)
}

func contains(item map[string]string, key string) bool {
	if _, ok := item[key]; ok {
		return true
	}
	return false
}

func defaultValue(args ...any) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("default called with no values")
	}

	if len(args) > 0 {
		if args[0] != nil {
			if s, ok := args[0].(string); ok {
				return s, nil
			}
		}
	}

	if len(args) > 1 {
		if args[1] == nil {
			return "", fmt.Errorf("default called with nil default value")
		}

		if _, ok := args[1].(string); !ok {
			return "", fmt.Errorf("default is not a string value, hint: surround it with double quotes")
		}

		return args[1].(string), nil
	}

	return "", fmt.Errorf("default called with no default value")
}

func parseUrl(rawurl string) (*url.URL, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url %q: %w", rawurl, err)
	}
	return u, nil
}

func add(arg1, arg2 int) int {
	return arg1 + arg2
}

func isTrue(s string) bool {
	lower := strings.ToLower(s)
	switch lower {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		b, err := strconv.ParseBool(lower)
		if err == nil {
			return b
		}
		return false
	}
}

func jsonQuery(jsonObj string, query string) (any, error) {
	parser, err := gojq.NewStringQuery(jsonObj)
	if err != nil {
		return "", fmt.Errorf("unable to parse JSON object %s: %w", jsonObj, err)
	}
	res, err := parser.Query(query)
	if err != nil {
		return "", fmt.Errorf("unable to query JSON object %s with %s: %w", jsonObj, query, err)
	}
	return res, nil
}

// loop streams start, start+step, ... while the value is below stop. It takes
// 1-3 arguments (stop; start, stop; start, stop, step) and returns an error
// when step is not positive, so an error is returned for loop(5, 0, -1) too.
func loop(args ...int) (<-chan int, error) {
	var start, stop, step int
	switch len(args) {
	case 1:
		start, stop, step = 0, args[0], 1
	case 2:
		start, stop, step = args[0], args[1], 1
	case 3:
		start, stop, step = args[0], args[1], args[2]
	default:
		return nil, fmt.Errorf("wrong number of arguments, expected 1-3"+
			", but got %d", len(args))
	}
	// A step <= 0 would never advance (or, when negative, run until the int
	// overflows), so reject it before the producer goroutine starts.
	if step <= 0 {
		return nil, fmt.Errorf("loop: step must be positive, got %d", step)
	}

	c := make(chan int, 16)
	go func() {
		for i := start; i < stop; i += step {
			c <- i
		}
		close(c)
	}()
	return c, nil
}

// templateFuncMap returns sprig's functions with dockerize's own functions
// merged over them, so a dockerize function wins when both define the same name.
func templateFuncMap() template.FuncMap {
	templateMap := template.FuncMap{
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

	combinedFuncMap := sprig.TxtFuncMap()
	maps.Copy(combinedFuncMap, templateMap)
	return combinedFuncMap
}

func generateFile(templatePath, destPath string) bool {
	tmpl := template.New(filepath.Base(templatePath)).Funcs(templateFuncMap())

	if len(delims) > 0 {
		tmpl = tmpl.Delims(delims[0], delims[1])
	}
	tmpl, err := tmpl.ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("unable to parse template %s, error: %s", templatePath, err)
	}

	// Don't overwrite destination file if it exists and no-overwrite flag passed
	if _, err := os.Stat(destPath); err == nil && noOverwriteFlag {
		return false
	}

	// Render into memory first so a failed render never touches the destination.
	var out bytes.Buffer
	err = tmpl.ExecuteTemplate(&out, filepath.Base(templatePath), &Context{})
	if err != nil {
		log.Fatalf("template error %s, error: %s\n", templatePath, err)
	}

	if destPath == "" {
		if _, err := out.WriteTo(os.Stdout); err != nil {
			log.Fatalf("unable to write to stdout, error: %s", err)
		}
		return true
	}

	if err := writeFileAtomic(destPath, out.Bytes()); err != nil {
		log.Fatalf("unable to write %s, error: %s", destPath, err)
	}

	return true
}

// writeFileAtomic replaces destPath with data. It writes a temp file in the same
// directory and renames it over destPath, so a reader never sees a partial file.
// An existing destPath keeps its mode and owner; a new file gets 0666 masked by
// the umask, as os.Create would give it.
func writeFileAtomic(destPath string, data []byte) error {
	mode := os.FileMode(0o666) &^ currentUmask()
	uid, gid := -1, -1 // -1 leaves the owner unchanged

	fi, err := os.Stat(destPath)
	switch {
	case err == nil:
		mode = fi.Mode()
		st := fi.Sys().(*syscall.Stat_t)
		uid, gid = int(st.Uid), int(st.Gid)
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("unable to stat %s: %w", destPath, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("unable to create temp file for %s: %w", destPath, err)
	}
	// Removes the temp file on any failure. After a successful rename the name is gone and this is a no-op.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("unable to write temp file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("unable to chmod temp file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Chown(uid, gid); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("unable to chown temp file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("unable to close temp file %s: %w", tmp.Name(), err)
	}

	return os.Rename(tmp.Name(), destPath)
}

// currentUmask returns the process umask. syscall.Umask has no getter, so it
// sets a zero mask and restores the old one.
func currentUmask() os.FileMode {
	m := syscall.Umask(0)
	syscall.Umask(m)
	return os.FileMode(m)
}

func generateDir(templateDir, destDir string) bool {
	if destDir != "" {
		fiDest, err := os.Stat(destDir)
		if err != nil {
			log.Fatalf("unable to stat %s, error: %s", destDir, err)
		}
		if !fiDest.IsDir() {
			log.Fatalf("if template is a directory, dest must also be a directory (or stdout)")
		}
	}

	files, err := os.ReadDir(templateDir)
	if err != nil {
		log.Fatalf("bad directory: %s, error: %s", templateDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			log.Printf("skipping directory %s in template dir %s", file.Name(), templateDir)
			continue
		}
		if destDir == "" {
			generateFile(filepath.Join(templateDir, file.Name()), "")
		} else {
			generateFile(filepath.Join(templateDir, file.Name()), filepath.Join(destDir, file.Name()))
		}
	}

	return true
}
