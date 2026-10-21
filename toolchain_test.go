package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	goModToolchainRe = regexp.MustCompile(`(?m)^\s*toolchain\s+go(\S+)`)
	goModGoRe        = regexp.MustCompile(`(?m)^\s*go\s+(\S+)`)
	dockerfileGoRe   = regexp.MustCompile(`(?im)^\s*FROM\s+golang:([^\s-]+)`)
)

// goModVersion returns the toolchain directive from go.mod content if present,
// otherwise the go directive.
func goModVersion(content string) (string, bool) {
	if m := goModToolchainRe.FindStringSubmatch(content); m != nil {
		return m[1], true
	}
	if m := goModGoRe.FindStringSubmatch(content); m != nil {
		return m[1], true
	}
	return "", false
}

// dockerfileGoVersion returns the version part of the first golang image tag
// in the Dockerfile content, dropping any suffix such as "-alpine".
func dockerfileGoVersion(content string) (string, bool) {
	if m := dockerfileGoRe.FindStringSubmatch(content); m != nil {
		return m[1], true
	}
	return "", false
}

func TestDockerfileGoVersionMatchesGoMod(t *testing.T) {
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}

	modVersion, ok := goModVersion(string(goMod))
	if !ok {
		t.Fatal("no go or toolchain directive found in go.mod")
	}
	imageVersion, ok := dockerfileGoVersion(string(dockerfile))
	if !ok {
		t.Fatal("no FROM golang:<tag> line found in Dockerfile")
	}
	if modVersion != imageVersion {
		t.Errorf("Dockerfile uses golang:%s but go.mod requires %s", imageVersion, modVersion)
	}
}

func TestGoModVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{"go directive", "module x\n\ngo 1.27.3\n", "1.27.3", true},
		{"toolchain overrides go", "module x\n\ngo 1.27.0\ntoolchain go1.27.3\n", "1.27.3", true},
		{"no directive", "module x\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := goModVersion(tt.content)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("goModVersion() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestDockerfileGoVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{"plain tag", "FROM golang:1.27.3 AS binary\n", "1.27.3", true},
		{"alpine suffix", "FROM golang:1.27.3-alpine AS binary\n", "1.27.3", true},
		{"lowercase keyword", "from golang:1.27.3\n", "1.27.3", true},
		{"no golang image", "FROM gcr.io/distroless/static:nonroot\n", "", false},
		{"golang mentioned mid-line", "# uses golang:1.27.3\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := dockerfileGoVersion(strings.TrimSpace(tt.content))
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("dockerfileGoVersion() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
