package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestSliceVarString(t *testing.T) {
	var sv sliceVar
	sv.Set("test1")
	sv.Set("test2")
	result := sv.String()
	assert.Equal(t, "test1,test2", result)
}

func TestHostFlagsVarString(t *testing.T) {
	var hf hostFlagsVar
	hf.Set("host1")
	hf.Set("host2")
	result := hf.String()
	assert.Equal(t, "[host1 host2]", result)
}

func TestExists(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-exists")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name())

	existsResult, err := exists(tempFile.Name())
	assert.NoError(t, err)
	assert.True(t, existsResult)

	nonExisting := "/path/that/does/not/exist"
	existsResult, err = exists(nonExisting)
	assert.NoError(t, err)
	assert.False(t, existsResult)
}

func TestContains(t *testing.T) {
	testMap := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	assert.True(t, contains(testMap, "key1"))
	assert.True(t, contains(testMap, "key2"))
	assert.False(t, contains(testMap, "key3"))
}

func TestDefaultValue(t *testing.T) {
	result, err := defaultValue("test-value")
	assert.NoError(t, err)
	assert.Equal(t, "test-value", result)

	result, err = defaultValue("test-value", "default-value")
	assert.NoError(t, err)
	assert.Equal(t, "test-value", result)

	result, err = defaultValue(nil, "default-value")
	assert.NoError(t, err)
	assert.Equal(t, "default-value", result)

	_, err = defaultValue(nil, nil)
	assert.Error(t, err)

	_, err = defaultValue()
	assert.Error(t, err)
}

func TestParseUrl(t *testing.T) {
	url := parseUrl("http://example.com/path")
	assert.Equal(t, "http", url.Scheme)
	assert.Equal(t, "example.com", url.Host)
	assert.Equal(t, "/path", url.Path)

	url = parseUrl("https://api.example.com:8080/v1/users")
	assert.Equal(t, "https", url.Scheme)
	assert.Equal(t, "api.example.com:8080", url.Host)
	assert.Equal(t, "/v1/users", url.Path)
}

func TestAdd(t *testing.T) {
	result := add(5, 3)
	assert.Equal(t, 8, result)

	result = add(-1, 1)
	assert.Equal(t, 0, result)

	result = add(0, 0)
	assert.Equal(t, 0, result)
}

func TestIsTrue(t *testing.T) {
	assert.True(t, isTrue("true"))
	assert.True(t, isTrue("TRUE"))
	assert.True(t, isTrue("1"))
	assert.True(t, isTrue("yes"))
	assert.True(t, isTrue("on"))

	assert.False(t, isTrue("false"))
	assert.False(t, isTrue("FALSE"))
	assert.False(t, isTrue("0"))
	assert.False(t, isTrue("no"))
	assert.False(t, isTrue("off"))
	assert.False(t, isTrue(""))
	assert.False(t, isTrue("invalid"))
}

func TestJSONQuery(t *testing.T) {
	jsonDoc := `{"services":[{"name":"service1","port":8000},{"name":"service2","port":9000}]}`

	// Test extracting array
	result, err := jsonQuery(jsonDoc, "services")
	assert.NoError(t, err)
	assert.Len(t, result.([]interface{}), 2)

	// Test extracting a value
	portResult, err := jsonQuery(jsonDoc, "services.[0].port")
	assert.NoError(t, err)
	assert.Equal(t, float64(8000), portResult)

	portResult, err = jsonQuery(jsonDoc, ".services.[1].port")
	assert.NoError(t, err)
	assert.Equal(t, float64(9000), portResult)
	
	// Test error cases
	_, err = jsonQuery("not-json", ".")
	assert.Error(t, err)
	_, err = jsonQuery(jsonDoc, "")
	assert.Error(t, err)
}

func TestLoop(t *testing.T) {
	ch, err := loop(3)
	assert.NoError(t, err)

	var result []int
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{0, 1, 2}, result)

	ch, err = loop(2, 5)
	assert.NoError(t, err)

	result = []int{}
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{2, 3, 4}, result)

	ch, err = loop(0, 10, 2)
	assert.NoError(t, err)

	result = []int{}
	for i := range ch {
		result = append(result, i)
	}
	assert.Equal(t, []int{0, 2, 4, 6, 8}, result)

	_, err = loop()
	assert.Error(t, err)

	_, err = loop(1, 2, 3, 4)
	assert.Error(t, err)
}
