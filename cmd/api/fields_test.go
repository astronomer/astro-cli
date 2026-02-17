package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFields(t *testing.T) {
	tests := []struct {
		name        string
		magicFields []string
		rawFields   []string
		expected    map[string]interface{}
		expectError bool
	}{
		{
			name:        "empty fields",
			magicFields: nil,
			rawFields:   nil,
			expected:    map[string]interface{}{},
		},
		{
			name:        "simple raw field",
			magicFields: nil,
			rawFields:   []string{"name=test"},
			expected:    map[string]interface{}{"name": "test"},
		},
		{
			name:        "multiple raw fields",
			magicFields: nil,
			rawFields:   []string{"name=test", "description=a description"},
			expected:    map[string]interface{}{"name": "test", "description": "a description"},
		},
		{
			name:        "magic field with integer",
			magicFields: []string{"count=42"},
			rawFields:   nil,
			expected:    map[string]interface{}{"count": 42},
		},
		{
			name:        "magic field with boolean true",
			magicFields: []string{"enabled=true"},
			rawFields:   nil,
			expected:    map[string]interface{}{"enabled": true},
		},
		{
			name:        "magic field with boolean false",
			magicFields: []string{"enabled=false"},
			rawFields:   nil,
			expected:    map[string]interface{}{"enabled": false},
		},
		{
			name:        "magic field with null",
			magicFields: []string{"value=null"},
			rawFields:   nil,
			expected:    map[string]interface{}{"value": nil},
		},
		{
			name:        "magic field with string that looks like int",
			magicFields: []string{"version=1.0"},
			rawFields:   nil,
			expected:    map[string]interface{}{"version": "1.0"},
		},
		{
			name:        "raw field preserves int-like string",
			magicFields: nil,
			rawFields:   []string{"count=42"},
			expected:    map[string]interface{}{"count": "42"},
		},
		{
			name:        "nested field",
			magicFields: nil,
			rawFields:   []string{"config[key]=value"},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"key": "value",
				},
			},
		},
		{
			name:        "deeply nested field",
			magicFields: nil,
			rawFields:   []string{"config[level1][level2]=value"},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"level1": map[string]interface{}{
						"level2": "value",
					},
				},
			},
		},
		{
			name:        "array field",
			magicFields: nil,
			rawFields:   []string{"tags[]=one", "tags[]=two"},
			expected: map[string]interface{}{
				"tags": []interface{}{"one", "two"},
			},
		},
		{
			name:        "empty array",
			magicFields: nil,
			rawFields:   []string{"tags[]"},
			expected: map[string]interface{}{
				"tags": []interface{}{},
			},
		},
		{
			name:        "missing value",
			magicFields: nil,
			rawFields:   []string{"name"},
			expectError: true,
		},
		{
			name:        "mixed raw and magic fields",
			magicFields: []string{"count=42", "enabled=true"},
			rawFields:   []string{"name=test"},
			expected:    map[string]interface{}{"name": "test", "count": 42, "enabled": true},
		},
		{
			name:        "duplicate key error",
			magicFields: nil,
			rawFields:   []string{"name=first", "name=second"},
			expectError: true,
		},
		{
			name:        "array of objects",
			magicFields: nil,
			rawFields:   []string{"items[][name]=one", "items[][name]=two"},
			expected: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "one"},
					map[string]interface{}{"name": "two"},
				},
			},
		},
		{
			name:        "type conflict map over array",
			magicFields: nil,
			rawFields:   []string{"tags[]=one", "tags[key]=val"},
			expectError: true,
		},
		{
			name:        "type conflict array over map",
			magicFields: nil,
			rawFields:   []string{"config[key]=val", "config[]=item"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseFields(tt.magicFields, tt.rawFields)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMagicFieldValue(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"true", true},
		{"false", false},
		{"null", nil},
		{"42", 42},
		{"-10", -10},
		{"0", 0},
		{"hello", "hello"},
		{"1.5", "1.5"},     // floats are kept as strings
		{"true1", "true1"}, // not exactly true
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := magicFieldValue(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMagicFieldValueFileReading(t *testing.T) {
	// Create a temp file with known content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "input.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("file content"), 0o600))

	t.Run("reads file with @ prefix", func(t *testing.T) {
		result, err := magicFieldValue("@" + tmpFile)
		require.NoError(t, err)
		assert.Equal(t, "file content", result)
	})

	t.Run("file not found error", func(t *testing.T) {
		_, err := magicFieldValue("@/nonexistent/path/file.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "opening file")
	})
}

func TestParseFieldsMagicFileError(t *testing.T) {
	// Exercises the "error parsing %q value" wrapping in parseField
	_, err := parseFields([]string{"data=@/nonexistent/file"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing")
}
