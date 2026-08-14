package codepush

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeaderMap_UnmarshalJSON_Object(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":{"Content-Type":"application/zip","x-custom":"value"}}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Equal(t, "application/zip", resp.Headers["Content-Type"])
	assert.Equal(t, "value", resp.Headers["x-custom"])
}

func TestHeaderMap_UnmarshalJSON_Array(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":[{"key":"Content-Type","value":"application/zip"},{"key":"x-custom","value":"value"}]}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Equal(t, "application/zip", resp.Headers["Content-Type"])
	assert.Equal(t, "value", resp.Headers["x-custom"])
}

func TestHeaderMap_UnmarshalJSON_Null(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":null}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Nil(t, resp.Headers)
}

func TestHeaderMap_UnmarshalJSON_EmptyArray(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":[]}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Empty(t, resp.Headers)
}

func TestHeaderMap_UnmarshalJSON_ArrayWithName(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":[{"name":"Content-Type","value":"application/zip"},{"name":"x-custom","value":"value"}]}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Equal(t, "application/zip", resp.Headers["Content-Type"])
	assert.Equal(t, "value", resp.Headers["x-custom"])
}

func TestHeaderMap_UnmarshalJSON_SkipsEmptyKeys(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":[{"key":"Content-Type","value":"application/zip"},{"key":"","value":"orphan"}]}`

	var resp UploadURLResponse
	require.NoError(t, json.Unmarshal([]byte(data), &resp))

	assert.Len(t, resp.Headers, 1)
	assert.Equal(t, "application/zip", resp.Headers["Content-Type"])
}

func TestHeaderMap_UnmarshalJSON_Invalid(t *testing.T) {
	data := `{"url":"https://example.com","method":"PUT","headers":"bad"}`

	var resp UploadURLResponse
	require.Error(t, json.Unmarshal([]byte(data), &resp))
}
