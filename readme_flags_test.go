package main

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeDocumentsFlags(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}

	for _, flag := range []string{"-version", "-wait-retry-interval"} {
		if !strings.Contains(string(readme), "`"+flag+"`") {
			t.Errorf("README.md does not document the %s flag", flag)
		}
	}
}
