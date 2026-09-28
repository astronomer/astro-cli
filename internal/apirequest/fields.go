package apirequest

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	keyStart     = '['
	keyEnd       = ']'
	keySeparator = '='
)

// Numbers says which -F values are sent as JSON numbers rather than strings.
// The two raw-API families disagree, and each keeps its own rule: `astro api`
// follows gh, which reads only integers, while `astro local api` follows the
// standalone af, which reads any number Python's float() would.
type Numbers int

const (
	// Integers reads 42 as a number and keeps 1.5 a string, gh's rule. A
	// version like 3.0 survives as the string it was typed as.
	Integers Numbers = iota
	// AnyNumber also reads 1.5 and 1e3 as numbers, af's rule. Only finite
	// decimal values count: af's Python reads "nan" and "inf" as floats and
	// then sends JSON no server parses, and never reads a hex literal at all.
	AnyNumber
)

// Fields is one command's -F and -f flags.
type Fields struct {
	// Magic are the -F values, typed: true, false, null, numbers, and @file.
	Magic []string
	// Raw are the -f values, always strings.
	Raw []string
	// Numbers is which values in Magic read as numbers.
	Numbers Numbers
	// Stdin is what @- reads. Nil reads the process's stdin.
	Stdin io.Reader
}

// Parse turns the fields into the request parameters: a JSON body for a
// method that carries one, or a query string for one that does not.
//
// Raw fields are parsed first and typed fields after them, and naming one key
// twice is an error rather than a precedence rule. key[sub]=v nests an object
// and key[]=v appends to an array.
func (f Fields) Parse() (map[string]any, error) {
	params := make(map[string]any)
	for _, raw := range f.Raw {
		if err := f.parseField(params, raw, false); err != nil {
			return params, err
		}
	}
	for _, magic := range f.Magic {
		if err := f.parseField(params, magic, true); err != nil {
			return params, err
		}
	}
	return params, nil
}

// ParseFields parses -F and -f under gh's rule for numbers, for the commands
// under `astro api`.
func ParseFields(magicFields, rawFields []string) (map[string]any, error) {
	return Fields{Magic: magicFields, Raw: rawFields}.Parse()
}

// parseField parses a single field and adds it to the params map.
//
//nolint:gocognit // Complex parsing logic for nested field syntax
func (f Fields) parseField(params map[string]any, field string, isMagic bool) error {
	var valueIndex int
	var keystack []string
	keyStartAt := 0

parseLoop:
	for i, r := range field {
		switch r {
		case keyStart:
			if keyStartAt == 0 {
				keystack = append(keystack, field[0:i])
			}
			keyStartAt = i + 1
		case keyEnd:
			keystack = append(keystack, field[keyStartAt:i])
		case keySeparator:
			if keyStartAt == 0 {
				keystack = append(keystack, field[0:i])
			}
			valueIndex = i + 1
			break parseLoop
		}
	}

	if len(keystack) == 0 {
		return fmt.Errorf("invalid key: %q", field)
	}

	key := field
	var value any
	if valueIndex == 0 {
		if keystack[len(keystack)-1] != "" {
			return fmt.Errorf("field %q requires a value separated by an '=' sign", key)
		}
		// Empty array notation: key[]
		value = nil
	} else {
		key = field[0 : valueIndex-1]
		value = field[valueIndex:]
	}

	if isMagic && value != nil {
		var err error
		value, err = f.magicValue(value.(string))
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
			destMap[subkey] = []any{}
		} else {
			if v, exists := destMap[subkey]; exists {
				if existSlice, ok := v.([]any); ok {
					destMap[subkey] = append(existSlice, value)
				} else {
					return fmt.Errorf("expected array type under %q, got %T", subkey, v)
				}
			} else {
				destMap[subkey] = []any{value}
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
func addParamsMap(m map[string]any, key string) (map[string]any, error) {
	if v, exists := m[key]; exists {
		if existMap, ok := v.(map[string]any); ok {
			return existMap, nil
		}
		return nil, fmt.Errorf("expected map type under %q, got %T", key, v)
	}
	newMap := make(map[string]any)
	m[key] = newMap
	return newMap, nil
}

// addParamsSlice handles adding to an array of objects.
func addParamsSlice(m map[string]any, prevkey, newkey string) (map[string]any, error) {
	if v, exists := m[prevkey]; exists {
		if existSlice, ok := v.([]any); ok {
			if len(existSlice) > 0 {
				lastItem := existSlice[len(existSlice)-1]
				if lastMap, ok := lastItem.(map[string]any); ok {
					if _, keyExists := lastMap[newkey]; !keyExists {
						return lastMap, nil
					}
				}
			}
			newMap := make(map[string]any)
			m[prevkey] = append(existSlice, newMap)
			return newMap, nil
		}
		return nil, fmt.Errorf("expected array type under %q, got %T", prevkey, v)
	}
	newMap := make(map[string]any)
	m[prevkey] = []any{newMap}
	return newMap, nil
}

// magicValue converts one -F value to its type: @file reads a file (@- reads
// stdin), true, false and null are themselves, and a number is a number by
// this field set's rule. Anything else stays the string it was.
func (f Fields) magicValue(v string) (any, error) {
	if strings.HasPrefix(v, "@") {
		return f.readFileValue(v[1:])
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}
	if f.Numbers == AnyNumber {
		if n, ok := decimalNumber(v); ok {
			return n, nil
		}
	}
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

// decimalNumber reads a finite decimal float. strconv also takes hex floats,
// infinities and NaN, which af leaves as strings or sends as JSON nothing can
// parse; neither is a number anyone meant to type.
func decimalNumber(v string) (float64, bool) {
	if strings.ContainsAny(v, "xX") {
		return 0, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

// readFileValue reads a value from a file or stdin.
func (f Fields) readFileValue(filename string) (string, error) {
	var r io.Reader
	if filename == "-" {
		r = f.Stdin
		if r == nil {
			r = os.Stdin
		}
	} else {
		file, err := os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("opening file %q: %w", filename, err)
		}
		defer file.Close()
		r = file
	}

	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	return string(b), nil
}
