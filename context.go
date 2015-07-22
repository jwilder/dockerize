package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

type Context struct {
	facter map[string]string
}

func (c *Context) Env() map[string]string {
	env := make(map[string]string)
	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		env[i[0:sep]] = i[sep+1:]
	}
	return env
}

func (c *Context) Facter() map[string]string {
	if c.facter != nil {
		return c.facter
	}
	c.facter = make(map[string]string)
	cmd := exec.Command("facter")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return c.facter
	}
	scanner := bufio.NewScanner(stdout)
	go func() {
		for scanner.Scan() {
			fact := strings.Split(scanner.Text(), " ")
			c.facter[fact[0]] = fact[2]
		}
	}()
	if err := cmd.Run(); err != nil {
		return c.facter
	}
	return c.facter
}
