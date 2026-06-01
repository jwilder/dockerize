package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplateContains(t *testing.T) {
	m := map[string]string{"key": "val"}
	assert.True(t, contains(m, "key"))
	assert.False(t, contains(m, "missing"))
}

func TestTemplateDefaultValue(t *testing.T) {
	val, err := defaultValue("hello")
	assert.NoError(t, err)
	assert.Equal(t, "hello", val)

	val, err = defaultValue(nil, "fallback")
	assert.NoError(t, err)
	assert.Equal(t, "fallback", val)

	_, err = defaultValue()
	assert.Error(t, err)

	_, err = defaultValue(nil)
	assert.Error(t, err)

	_, err = defaultValue(nil, nil)
	assert.Error(t, err)

	_, err = defaultValue(nil, 123)
	assert.Error(t, err)
}

func TestTemplateParseUrl(t *testing.T) {
	u := parseUrl("http://host:8080/path")
	assert.Equal(t, "host:8080", u.Host)
	assert.Equal(t, "/path", u.Path)
}

func TestTemplateAdd(t *testing.T) {
	assert.Equal(t, 5, add(2, 3))
}

func TestIsTrue(t *testing.T) {
	for _, s := range []string{"true", "1", "yes", "on", "True", "YES"} {
		assert.True(t, isTrue(s))
	}
	for _, s := range []string{"false", "0", "no", "off", "False", "NO"} {
		assert.False(t, isTrue(s))
	}
	assert.False(t, isTrue("random"))
}

func TestJsonQuery(t *testing.T) {
	res, err := jsonQuery(`{"name":"test"}`, ".name")
	assert.NoError(t, err)
	assert.Equal(t, "test", res)

	_, err = jsonQuery("invalid", ".foo")
	assert.Error(t, err)
}

func TestLoop(t *testing.T) {
	c, err := loop(3)
	assert.NoError(t, err)
	var vals []int
	for v := range c {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{0, 1, 2}, vals)

	c, err = loop(1, 4)
	assert.NoError(t, err)
	vals = nil
	for v := range c {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{1, 2, 3}, vals)

	c, err = loop(0, 6, 2)
	assert.NoError(t, err)
	vals = nil
	for v := range c {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{0, 2, 4}, vals)

	_, err = loop()
	assert.Error(t, err)
}

func TestExists(t *testing.T) {
	ok, err := exists("template.go")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = exists("nonexistent_file")
	assert.NoError(t, err)
	assert.False(t, ok)
}