package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// I use this instead of base testing Suite
	// to bring back warm fuzzies of junit
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type MySuite struct {
	dir string
}

var _ = Suite(&MySuite{})

func (s *MySuite) SetUpSuite(c *C) {
	os.Setenv("VAL_1", "one")
	os.Setenv("VAL_2", "two")
	s.dir = c.MkDir()
}

func (s *MySuite) TestJsonTemplate(c *C) {

	templatePath := filepath.Join("examples", "json", "json-example")
	answerFile := filepath.Join("test", "expected", "rendered-json-example")
	destPath := filepath.Join(s.dir, "output-json-example")
	generateFile(templatePath, destPath)
	fileCompare(answerFile, destPath, c)
}

// Test walking through a tree of templates
func (s *MySuite) TestDirTemplates(c *C) {
	dirpath := filepath.Join("test", "fixtures")
	generateDir(dirpath, s.dir)

	fileCompare(filepath.Join("test", "expected", "one.conf"),
		filepath.Join(s.dir,
			"etc", "sample", "one.conf"), c)
	fileCompare(filepath.Join("test", "expected", "two.txt"),
		filepath.Join(s.dir,
			"etc", "conf", "two.txt"), c)

}

func fileCompare(expectedFile, actualFile string, c *C) {
	expectedResult, err := ioutil.ReadFile(expectedFile)
	if err != nil {
		c.Errorf("No file %s", expectedFile)
	}
	actualResult, err := ioutil.ReadFile(actualFile)
	if err != nil {
		c.Errorf("No file %s", actualFile)
	}
	c.Assert(strings.TrimSpace(string(actualResult)),
		Equals, strings.TrimSpace(string(expectedResult)))
}
