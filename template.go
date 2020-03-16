package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"

	"github.com/jwilder/gojq"
)

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func contains(item map[string]string, key string) bool {
	if _, ok := item[key]; ok {
		return true
	}
	return false
}

func _checkDefaultValueArgs(args interface{}) error {
	vargs, _ := args.([]interface{})

	if len(vargs) == 0 {
		return fmt.Errorf("default called with no values!")
	}

	if len(vargs) < 2 {
		return fmt.Errorf("default called with no default value")
	}

	if vargs[1] == nil {
		return fmt.Errorf("default called with nil default value!")
	}

	if _, ok := vargs[1].(string); !ok {
		return fmt.Errorf("default is not a string value. hint: surround it w/ double quotes.")
	}

	return nil
}

func defaultValue(args ...interface{}) (string, error) {
	if err := _checkDefaultValueArgs(args); err != nil {
		return "", err
	}

	if args[0] != nil {
		return args[0].(string), nil
	}

	return args[1].(string), nil
}

func defaultValueEmpty(args ...interface{}) (string, error) {
	if err := _checkDefaultValueArgs(args); err != nil {
		return "", err
	}

	if args[0] == nil || len(args[0].(string)) == 0 {
		return args[1].(string), nil
	}

	return args[0].(string), nil
}

func requiredValue(val interface{}) (interface{}, error) {
	if val != nil {
		return val, nil
	}

	return nil, fmt.Errorf("Required value is nil")
}

func requiredValueEmpty(val interface{}) (interface{}, error) {
	if _, err := requiredValue(val); err != nil {
		return nil, err
	}

	if val, ok := val.(string); ok && len(val) == 0 {
		return nil, fmt.Errorf("Required value is empty")
	}

	return val, nil
}

func parseUrl(rawurl string) *url.URL {
	u, err := url.Parse(rawurl)
	if err != nil {
		log.Fatalf("unable to parse url %s: %s", rawurl, err)
	}
	return u
}

func add(arg1, arg2 int) int {
	return arg1 + arg2
}

func isTrue(s string) bool {
	b, err := strconv.ParseBool(strings.ToLower(s))
	if err == nil {
		return b
	}
	return false
}

func jsonQuery(jsonObj string, query string) (interface{}, error) {
	parser, err := gojq.NewStringQuery(jsonObj)
	if err != nil {
		return "", err
	}
	res, err := parser.Query(query)
	if err != nil {
		return "", err
	}
	return res, nil
}

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
		return nil, fmt.Errorf("wrong number of arguments, expected 1-3" +
			", but got %d", len(args))
	}

	c := make(chan int)
	go func() {
		for i := start; i < stop; i += step {
			c <- i
		}
		close(c)
	}()
	return c, nil
}

func envSlice(envParamPrefix string) ([]map[string]interface{}, error) {
	variables := make(map[string]interface{})

	biggestIndex := 0

	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		key := i[0:sep]
		val := i[sep + 1:]

		if strings.HasPrefix(key, envParamPrefix) {
			suffix := key[strings.LastIndex(key, "_") + 1:]
			if num, ok := strconv.Atoi(suffix); ok == nil {
				if biggestIndex < num {
					biggestIndex = num
				}
				variables[key] = val
			}
		}
	}

	var result []map[string]interface{}
	if biggestIndex > 0 {
		result = make([]map[string]interface{}, biggestIndex)

		for i := 0; i < biggestIndex; i++ {
			result[i] = make(map[string]interface{})
		}

		for key, value := range variables {
			indexStr := key[strings.LastIndex(key, "_") + 1:]
			resultIndex, _ := strconv.Atoi(indexStr)
			resultIndex -= 1
			resultKey := key[0:strings.LastIndex(key, "_")]

			result[resultIndex][resultKey] = value
		}

	} else {
		result = make([]map[string]interface{}, 0)
	}

	return result, nil
}

func generateFile(templatePath, destPath string) bool {
	tmpl := template.New(filepath.Base(templatePath)).Funcs(template.FuncMap{
		"contains":      contains,
		"exists":        exists,
		"split":         strings.Split,
		"replace":       strings.Replace,
		"default":       defaultValue,
		"defaultEmpty":  defaultValueEmpty,
		"required":      requiredValue,
		"requiredEmpty": requiredValueEmpty,
		"parseUrl":      parseUrl,
		"atoi":          strconv.Atoi,
		"add":           add,
		"isTrue":        isTrue,
		"lower":         strings.ToLower,
		"upper":         strings.ToUpper,
		"jsonQuery":     jsonQuery,
		"loop":          loop,
		"envSlice":      envSlice,
	})

	if len(delims) > 0 {
		tmpl = tmpl.Delims(delims[0], delims[1])
	}
	tmpl, err := tmpl.ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("unable to parse template: %s", err)
	}

	// Don't overwrite destination file if it exists and no-overwrite flag passed
	if _, err := os.Stat(destPath); err == nil && noOverwriteFlag {
		return false
	}

	dest := os.Stdout
	if destPath != "" {
		dest, err = os.Create(destPath)
		if err != nil {
			log.Fatalf("unable to create %s", err)
		}
		defer dest.Close()
	}

	err = tmpl.ExecuteTemplate(dest, filepath.Base(templatePath), &Context{})
	if err != nil {
		log.Fatalf("template error: %s\n", err)
	}

	if fi, err := os.Stat(destPath); err == nil {
		if err := dest.Chmod(fi.Mode()); err != nil {
			log.Fatalf("unable to chmod temp file: %s\n", err)
		}
		if err := dest.Chown(int(fi.Sys().(*syscall.Stat_t).Uid), int(fi.Sys().(*syscall.Stat_t).Gid)); err != nil {
			log.Fatalf("unable to chown temp file: %s\n", err)
		}
	}

	return true
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

	files, err := ioutil.ReadDir(templateDir)
	if err != nil {
		log.Fatalf("bad directory: %s, error: %s", templateDir, err)
	}

	for _, file := range files {
		if destDir == "" {
			generateFile(filepath.Join(templateDir, file.Name()), "")
		} else {
			generateFile(filepath.Join(templateDir, file.Name()), filepath.Join(destDir, file.Name()))
		}
	}

	return true
}
