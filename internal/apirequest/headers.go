package apirequest

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/fatih/color"
)

// ParseHeaders reads -H values, each "key:value". Whitespace around both
// halves is trimmed, and a key given twice keeps the last value, which is what
// setting a header twice on a request does.
func ParseHeaders(headers []string) (http.Header, error) {
	parsed := http.Header{}
	for _, h := range headers {
		key, value, found := strings.Cut(h, ":")
		if !found {
			return nil, fmt.Errorf("invalid header format %q, expected key:value", h)
		}
		parsed.Set(strings.TrimSpace(key), strings.TrimSpace(value))
	}
	return parsed, nil
}

// WriteHead writes the block -i prints ahead of a body: the status line, the
// response headers sorted by name, and the blank line that ends them, the
// way an HTTP response arrives.
func WriteHead(w io.Writer, statusLine string, headers http.Header, colorize bool) {
	fmt.Fprintln(w, statusLine)
	writeHeaders(w, headers, colorize)
	fmt.Fprintln(w)
}

// writeHeaders prints HTTP headers sorted by name, one per line. A Status
// pseudo-header, which some servers echo, is left out: the status line has
// already said it.
func writeHeaders(w io.Writer, headers http.Header, colorize bool) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		if name == "Status" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	name := color.New(color.Bold, color.FgBlue)
	// Forced on: colorize already says whether this writer takes color, and
	// fatih/color would otherwise second-guess it from its own terminal check.
	name.EnableColor()
	for _, n := range names {
		if colorize {
			fmt.Fprintf(w, "%s: %s\n", name.Sprint(n), strings.Join(headers[n], ", "))
		} else {
			fmt.Fprintf(w, "%s: %s\n", n, strings.Join(headers[n], ", "))
		}
	}
}

// Query encodes the fields as a query string, the form they take on a GET.
// A nested object is key[sub]=v and an array key[]=v, the same syntax -F takes
// them in.
func Query(params map[string]any) url.Values {
	q := url.Values{}
	for key, value := range params {
		AddQuery(q, key, value)
	}
	return q
}

// AddQuery adds one field to a query, recursing into objects and arrays.
func AddQuery(q url.Values, key string, value any) {
	switch v := value.(type) {
	case string:
		q.Add(key, v)
	case int:
		q.Add(key, fmt.Sprintf("%d", v))
	case bool:
		q.Add(key, fmt.Sprintf("%v", v))
	case nil:
		q.Add(key, "")
	case []any:
		for _, item := range v {
			AddQuery(q, key+"[]", item)
		}
	case map[string]any:
		for subkey, subvalue := range v {
			AddQuery(q, key+"["+subkey+"]", subvalue)
		}
	default:
		q.Add(key, fmt.Sprintf("%v", v))
	}
}
