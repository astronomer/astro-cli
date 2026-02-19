package api

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	keyStart     = '['
	keyEnd       = ']'
	keySeparator = '='
)

// parseFields parses magic fields and raw fields into a map.
// Magic fields (-F) support type conversion and file reading.
// Raw fields (-f) are always treated as strings.
func parseFields(magicFields, rawFields []string) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	// Parse raw fields first (string-only)
	for _, f := range rawFields {
		if err := parseField(params, f, false); err != nil {
			return params, err
		}
	}

	// Parse magic fields (with type conversion)
	for _, f := range magicFields {
		if err := parseField(params, f, true); err != nil {
			return params, err
		}
	}

	return params, nil
}

// parseField parses a single field and adds it to the params map.
//
//nolint:gocognit // Complex parsing logic for nested field syntax
func parseField(params map[string]interface{}, f string, isMagic bool) error {
	var valueIndex int
	var keystack []string
	keyStartAt := 0

parseLoop:
	for i, r := range f {
		switch r {
		case keyStart:
			if keyStartAt == 0 {
				keystack = append(keystack, f[0:i])
			}
			keyStartAt = i + 1
		case keyEnd:
			keystack = append(keystack, f[keyStartAt:i])
		case keySeparator:
			if keyStartAt == 0 {
				keystack = append(keystack, f[0:i])
			}
			valueIndex = i + 1
			break parseLoop
		}
	}

	if len(keystack) == 0 {
		return fmt.Errorf("invalid key: %q", f)
	}

	key := f
	var value interface{}
	if valueIndex == 0 {
		if keystack[len(keystack)-1] != "" {
			return fmt.Errorf("field %q requires a value separated by an '=' sign", key)
		}
		// Empty array notation: key[]
		value = nil
	} else {
		key = f[0 : valueIndex-1]
		value = f[valueIndex:]
	}

	if isMagic && value != nil {
		var err error
		value, err = magicFieldValue(value.(string))
		if err != nil {
			return fmt.Errorf("error parsing %q value: %w", key, err)
		}
	}

	destMap := params
	isArray := false
	var subkey string

	for _, k := range keystack {
		if k == "" {
			isArray = true
			continue
		}
		if subkey != "" {
			var err error
			if isArray {
				destMap, err = addParamsSlice(destMap, subkey, k)
				isArray = false
			} else {
				destMap, err = addParamsMap(destMap, subkey)
			}
			if err != nil {
				return err
			}
		}
		subkey = k
	}

	if isArray {
		if value == nil {
			destMap[subkey] = []interface{}{}
		} else {
			if v, exists := destMap[subkey]; exists {
				if existSlice, ok := v.([]interface{}); ok {
					destMap[subkey] = append(existSlice, value)
				} else {
					return fmt.Errorf("expected array type under %q, got %T", subkey, v)
				}
			} else {
				destMap[subkey] = []interface{}{value}
			}
		}
	} else {
		if _, exists := destMap[subkey]; exists {
			return fmt.Errorf("unexpected override existing field under %q", subkey)
		}
		destMap[subkey] = value
	}

	return nil
}

// addParamsMap ensures a nested map exists at the given key and returns it.
func addParamsMap(m map[string]interface{}, key string) (map[string]interface{}, error) {
	if v, exists := m[key]; exists {
		if existMap, ok := v.(map[string]interface{}); ok {
			return existMap, nil
		}
		return nil, fmt.Errorf("expected map type under %q, got %T", key, v)
	}
	newMap := make(map[string]interface{})
	m[key] = newMap
	return newMap, nil
}

// addParamsSlice handles adding to an array of objects.
func addParamsSlice(m map[string]interface{}, prevkey, newkey string) (map[string]interface{}, error) {
	if v, exists := m[prevkey]; exists {
		if existSlice, ok := v.([]interface{}); ok {
			if len(existSlice) > 0 {
				lastItem := existSlice[len(existSlice)-1]
				if lastMap, ok := lastItem.(map[string]interface{}); ok {
					if _, keyExists := lastMap[newkey]; !keyExists {
						return lastMap, nil
					}
				}
			}
			newMap := make(map[string]interface{})
			m[prevkey] = append(existSlice, newMap)
			return newMap, nil
		}
		return nil, fmt.Errorf("expected array type under %q, got %T", prevkey, v)
	}
	newMap := make(map[string]interface{})
	m[prevkey] = []interface{}{newMap}
	return newMap, nil
}

// magicFieldValue converts a string value to its appropriate type.
// Supports: true, false, null, integers, and file reading with @.
func magicFieldValue(v string) (interface{}, error) {
	// File reading
	if strings.HasPrefix(v, "@") {
		return readFileValue(v[1:])
	}

	// Integer conversion
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}

	// Boolean and null conversion
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return v, nil
	}
}

// readFileValue reads a value from a file or stdin.
func readFileValue(filename string) (string, error) {
	var r io.Reader
	if filename == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("opening file %q: %w", filename, err)
		}
		defer f.Close()
		r = f
	}

	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	return string(b), nil
}
