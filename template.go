package main

import (
	"fmt"
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

func defaultValue(args ...interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("default called with no values!")
	}

	if len(args) > 0 {
		if args[0] != nil {
			return args[0].(string), nil
		}
	}

	if len(args) > 1 {
		if args[1] == nil {
			return "", fmt.Errorf("default called with nil default value!")
		}

		if _, ok := args[1].(string); !ok {
			return "", fmt.Errorf("default is not a string value. hint: surround it w/ double quotes.")
		}

		return args[1].(string), nil
	}

	return "", fmt.Errorf("default called with no default value")
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

func generateFile(templatePath, destPath string) bool {
	tmpl := template.New(filepath.Base(templatePath)).Funcs(template.FuncMap{
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
	})

	if len(delims) > 0 {
		tmpl = tmpl.Delims(delims[0], delims[1])
	}
	tmpl, err := tmpl.ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("unable to parse template: %s", err)
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

// TODO - make this be able to be set from CLI?
var templateSuffix = ".tpl"

func processTemplates(sourceDirPath, destDirPath string) {

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatalf("Could not process %s: %s", sourceDirPath, err)
			return nil
		}
		if strings.HasSuffix(info.Name(), templateSuffix) {
			destFileName := strings.TrimSuffix(info.Name(), templateSuffix)

			relPath, err := filepath.Rel(sourceDirPath, path)
			if err != nil {
				log.Fatalf("Could not split %q into relative path with base %q",
					path, sourceDirPath)
			}
			dir, _ := filepath.Split(relPath)
			// if err != nil {
			// 	log.Fatalf("Could not get directory file split from %q", relPath)
			// }

			destDir := filepath.Join(destDirPath, dir)
			os.MkdirAll(destDir, 0755)
			destFileName = filepath.Join(destDir, destFileName)

			//Drop the first parth, which shoudl be the sourceDirPath
			//and the last part which is
			log.Println("writing to ", destFileName)

			generateFile(path, destFileName)

		}
		return nil
	}
	filepath.Walk(sourceDirPath, walker)

}
