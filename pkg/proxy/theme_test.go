package proxy

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const themeTag = `<style id="astro-theme">`

func newHTMLResponse(t *testing.T, body string, gzipped bool) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if gzipped {
		gw := gzip.NewWriter(&buf)
		_, _ = gw.Write([]byte(body))
		_ = gw.Close()
	} else {
		buf.WriteString(body)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.ContentLength = int64(buf.Len())
	if gzipped {
		resp.Header.Set("Content-Encoding", "gzip")
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		out, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("gzip read: %v", err)
		}
		_ = gr.Close()
		return string(out)
	}
	return string(body)
}

func TestInjectIntoHTMLBasicInjection(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head><title>x</title></head><body></body></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>foo</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got := readBody(t, resp)
	want := "<html><head><title>x</title><style>foo</style></head><body></body></html>"
	if got != want {
		t.Fatalf("body mismatch:\n got: %q\nwant: %q", got, want)
	}
	if resp.ContentLength != int64(len(want)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(want))
	}
	if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(len(want)) {
		t.Errorf("Content-Length header = %q, want %q", cl, strconv.Itoa(len(want)))
	}
}

func TestInjectIntoHTMLNonHTMLPassesThrough(t *testing.T) {
	const original = "not actually html, but body says so"
	resp := newHTMLResponse(t, original, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	resp.Header.Set("Content-Type", "application/json")

	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); got != original {
		t.Errorf("non-HTML body was modified: got %q, want %q", got, original)
	}
}

func TestInjectIntoHTMLNoMarkerIsNoOpOnContent(t *testing.T) {
	body := "<html><body>no head tag here</body></html>"
	resp := newHTMLResponse(t, body, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); got != body {
		t.Errorf("body changed when marker absent: got %q, want %q", got, body)
	}
}

// </head> anchors on its first occurrence. The real one closes the head near the
// top of the page; a later "</head>" is body content, an inline script's
// string or a rendered Dag doc, and the snippet must not land in it.
func TestInjectIntoHTMLAnchorsTheHeadOnItsFirstClose(t *testing.T) {
	page := `<html><head><title>t</title></head><body><script>var s = "</head>";</script></body></html>`
	resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	want := `<html><head><title>t</title><style>x</style></head><body><script>var s = "</head>";</script></body></html>`
	if got := readBody(t, resp); got != want {
		t.Errorf("snippet not before the first </head>:\n got: %q\nwant: %q", got, want)
	}
}

// Any other marker anchors on its last occurrence: </body> closes the page at
// the end, and an earlier "</body>" is a string in an inline script.
func TestInjectIntoHTMLAnchorsOtherMarkersOnTheirLast(t *testing.T) {
	page := `<html><head></head><body><script>var s = "</body>";</script></body></html>`
	resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<i>x</i>", "</body>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	want := `<html><head></head><body><script>var s = "</body>";</script><i>x</i></body></html>`
	if got := readBody(t, resp); got != want {
		t.Errorf("snippet not before the last </body>:\n got: %q\nwant: %q", got, want)
	}
}

func TestInjectIntoHTMLCaseInsensitiveMarker(t *testing.T) {
	resp := newHTMLResponse(t, "<HTML><HEAD></HEAD><BODY></BODY></HTML>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := readBody(t, resp); !strings.Contains(got, "<style>x</style></HEAD>") {
		t.Errorf("expected snippet before uppercase </HEAD>: got %q", got)
	}
}

func TestInjectIntoHTMLGzipRoundTrip(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head><body></body></html>", true) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding lost on gzip body: got %q", resp.Header.Get("Content-Encoding"))
	}
	want := "<html><head><style>x</style></head><body></body></html>"
	if got := readBody(t, resp); got != want {
		t.Errorf("decoded body mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// A non-gzip HTML response carrying a Content-Encoding header (a misconfigured
// upstream) is rewritten as plain bytes, so the header is cleared or a client
// would try to decode them.
func TestInjectIntoHTMLPlaintextStripsContentEncoding(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	resp.Header.Set("Content-Encoding", "identity")
	if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding not cleared on plaintext body: got %q", got)
	}
}

// A body that can't be read is an error, not a silent pass-through. By the time
// reading fails some of the body is gone, and passing on what's left under the
// original Content-Length hands the client an empty or truncated page with a
// 200. Returning the error lets the proxy answer with its error handler.
func TestInjectIntoHTMLReportsABodyItCannotRead(t *testing.T) {
	for name, tc := range map[string]struct {
		body     io.Reader
		encoding string
	}{
		"claims gzip but isn't": {body: strings.NewReader("not gzip data"), encoding: "gzip"},
		"fails mid-read":        {body: io.MultiReader(strings.NewReader("<html><head>"), errReader{}), encoding: ""},
	} {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(tc.body),
			}
			resp.Header.Set("Content-Type", "text/html")
			if tc.encoding != "" {
				resp.Header.Set("Content-Encoding", tc.encoding)
			}
			if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err == nil {
				t.Error("an unreadable body was passed on as if it were the page")
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset mid-body") }

func TestInjectAirflowThemeCarriesThePrePaintBackground(t *testing.T) {
	resp := newHTMLResponse(t, `<html><head><title>Airflow</title></head><body><div id="root"></div></body></html>`, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatalf("inject: %v", err)
	}
	got := readBody(t, resp)

	head := strings.Index(got, "</head>")
	script := strings.Index(got, "prefers-color-scheme: dark")
	if script < 0 || script > head {
		t.Errorf("pre-paint script not injected inside <head>:\n%s", got)
	}
	// Both ways the rule has to stop applying: the selector for Airflow 3,
	// which marks the root as next-themes boots, and the #root test for
	// Airflow 2, which never marks it and never needed the bridge.
	if !strings.Contains(got, "html:not(.light):not(.dark){background:") {
		t.Errorf("pre-paint rule is not scoped to the pre-boot root:\n%s", got)
	}
	if !strings.Contains(got, "if (!document.getElementById('root')) s.remove();") {
		t.Errorf("pre-paint rule never removes itself on a page with no #root:\n%s", got)
	}
	// Runtime's black and white tokens, which Airflow paints the page with
	// once the theme lands.
	for _, want := range []string{"oklch(0.220 0.025 289)", "oklch(0.985 0.002 289)"} {
		if !strings.Contains(got, "'"+want+"'") {
			t.Errorf("pre-paint script missing background %s:\n%s", want, got)
		}
	}
	// Shares the pass with the theme CSS rather than costing a second one.
	head = strings.Index(got, "</head>")
	if i := strings.Index(got, themeTag); i < 0 || i > head {
		t.Errorf("theme CSS is not in the head:\n%s", got)
	}
	if !strings.Contains(got, "--chakra-colors-brand-600: oklch(0.460 0.190 298.0);") {
		t.Errorf("theme CSS does not set Runtime's brand color:\n%s", got)
	}
}

// The colors are Runtime's theme and nothing else: its brand and gray palettes
// and its black and white, the same tokens Airflow 3.2+ takes from
// AIRFLOW__API__THEME on Astro. Airflow's own semantic and status colors stay
// Airflow's, as they do there.
func TestInjectAirflowThemeSetsRuntimesPalettes(t *testing.T) {
	resp := newHTMLResponse(t, `<html><head></head><body><div id="root"></div></body></html>`, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	got := readBody(t, resp)

	for _, want := range []string{
		"--chakra-colors-black: oklch(0.220 0.025 289);",
		"--chakra-colors-white: oklch(0.985 0.002 289);",
		"--chakra-colors-brand-50: oklch(0.975 0.003 289);",
		"--chakra-colors-brand-300: oklch(0.800 0.080 298.0);",
		"--chakra-colors-brand-800: oklch(0.328 0.035 289);",
		"--chakra-colors-gray-500: oklch(0.630 0.008 289);",
		"--chakra-colors-gray-950: oklch(0.160 0.018 289);",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("theme CSS is missing %q:\n%s", want, got)
		}
	}

	// Any variable name at all, so a stray one in capitals or digits is
	// counted and named too.
	vars := regexp.MustCompile(`--chakra-colors-([^:\s]+)`).FindAllStringSubmatch(got, -1)
	if len(vars) != 24 {
		t.Errorf("theme CSS sets %d colors, want Runtime's 24 (11 brand, 11 gray, black, white)", len(vars))
	}
	for _, v := range vars {
		palette, _, _ := strings.Cut(v[1], "-")
		switch palette {
		case "brand", "gray", "black", "white":
		default:
			t.Errorf("theme CSS sets %s, which Runtime's theme leaves to Airflow", v[0])
		}
	}

	// Scoped to the classes Airflow 3 puts on its root, so Airflow 2, which
	// Runtime does not theme, keeps its own colors.
	if !strings.Contains(got, "html.light, html.dark {\n  --chakra-colors-") {
		t.Errorf("theme colors are not scoped to Airflow 3's root:\n%s", got)
	}
}

// A scale's steps go out lightest to darkest. Sorted as strings, 100 would come
// before 50.
func TestParseAirflowThemeOrdersStepsNumerically(t *testing.T) {
	theme, err := parseAirflowTheme([]byte(`{"tokens":{"colors":{
		"black":{"value":"oklch(0.2 0 0)"},"white":{"value":"oklch(1 0 0)"},
		"brand":{"950":{"value":"oklch(0.1 0 0)"},"100":{"value":"oklch(0.9 0 0)"},"50":{"value":"oklch(0.95 0 0)"}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range theme.colors {
		names = append(names, v.name)
	}
	want := "--chakra-colors-black --chakra-colors-brand-50 --chakra-colors-brand-100 --chakra-colors-brand-950 --chakra-colors-white"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("colors in order\n got: %s\nwant: %s", got, want)
	}
}

// themeWith is a theme Airflow and the proxy both take, with colors added to
// its palette and rest added beside its tokens, so a case below breaks exactly
// one rule.
func themeWith(colors, rest string) string {
	return `{"tokens":{"colors":{"black":{"value":"oklch(0.2 0 0)"},"white":{"value":"oklch(1 0 0)"}` +
		colors + `}}` + rest + `}`
}

// The shape Runtime ships parses: an empty globalCss rule (its workaround for
// Airflow 3.2.0) and no icons.
func TestParseAirflowThemeTakesRuntimesShape(t *testing.T) {
	if _, err := parseAirflowTheme([]byte(themeWith(`,"brand":{"600":{"value":"oklch(0.46 0.19 298)"}}`, `,"globalCss":{"html":{}},"icon":null`))); err != nil {
		t.Fatal(err)
	}
}

// A theme is refused when Airflow would refuse it, and when Airflow would show
// something the proxy cannot paint. Either way a Runtime theme copied in that
// the proxy would render differently fails here, instead of local Airflow
// quietly drifting from Astro.
func TestParseAirflowThemeRefusesWhatItCannotPaintAsAirflowWould(t *testing.T) {
	for _, tc := range []struct{ name, doc, wantErr string }{
		{"not JSON", `{`, "parsing"},
		{"no colors", `{"globalCss":{"html":{}}}`, "no colors"},
		{"no black or white", `{"tokens":{"colors":{"brand":{"50":{"value":"oklch(0.9 0 0)"}}}}}`, "black and white"},
		// Airflow's own palettes are brand and gray; Chakra's others are not
		// part of a theme Airflow takes.
		{"another palette", themeWith(`,"blue":{"500":{"value":"oklch(0.5 0.2 250)"}}`, ""), `"blue"`},
		{"a camel-cased palette", themeWith(`,"brandAccent":{"500":{"value":"oklch(0.5 0.2 250)"}}`, ""), `"brandAccent"`},
		{"semantic tokens", themeWith("", `,"semanticTokens":{"colors":{}}`), `"semanticTokens"`},
		{"a step Airflow lacks", themeWith(`,"brand":{"solid":{"value":"oklch(0.5 0.2 250)"}}`, ""), `"solid"`},
		{"a step spelled another way", themeWith(`,"brand":{"050":{"value":"oklch(0.5 0.2 250)"}}`, ""), `"050"`},
		{"a step with no value", themeWith(`,"brand":{"50":{}}`, ""), "not an oklch() color"},
		{"a token reference", themeWith(`,"gray":{"900":{"value":"{colors.gray.800}"}}`, ""), "not an oklch() color"},
		{"a hex color", themeWith(`,"gray":{"900":{"value":"#123456"}}`, ""), "not an oklch() color"},
		{"a value that ends the style block", themeWith(`,"gray":{"900":{"value":"oklch(0 0 0)</style>"}}`, ""), "not an oklch() color"},
		{"a value that ends the script string", themeWith(`,"gray":{"900":{"value":"oklch(0 0 0)'"}}`, ""), "not an oklch() color"},
		{"a per-mode value", themeWith(`,"gray":{"900":{"value":{"base":"oklch(0 0 0)","_dark":"oklch(1 0 0)"}}}`, ""), "parsing"},
		{"a color that is neither a shade nor a scale", themeWith(`,"brand":"purple"`, ""), "parsing"},
		{"an icon", themeWith("", `,"icon":"https://example.com/logo.svg"`), "icon"},
		{"a dark-mode icon", themeWith("", `,"icon_dark_mode":"https://example.com/logo.svg"`), "icon_dark_mode"},
		{"a globalCss rule", themeWith("", `,"globalCss":{"html":{"fontFamily":"serif"}}`), "globalCss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAirflowTheme([]byte(tc.doc))
			if err == nil {
				t.Fatal("parsed without an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("refused for another reason than %q: %v", tc.wantErr, err)
			}
		})
	}
}

// A theme that does not parse leaves Airflow's pages as they are, rather than
// breaking every program that links this package as it starts.
func TestInjectAirflowThemeWithNoThemeLeavesThePageAlone(t *testing.T) {
	orig := airflowThemeSnippet
	airflowThemeSnippet = func() string { return "" }
	t.Cleanup(func() { airflowThemeSnippet = orig })

	const page = "<html><head></head><body></body></html>"
	resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if got := readBody(t, resp); got != page {
		t.Errorf("page changed with no theme to inject: %q", got)
	}
}

// A page that already carries the theme gets it once. The desktop injects it on
// its own iframe proxies as well, and whichever runs second must not stack a
// second copy on top.
func TestInjectAirflowThemeLeavesAThemedPageAlone(t *testing.T) {
	resp := newHTMLResponse(t, "<html><head></head><body></body></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readBody(t, resp), themeTag); n != 1 {
		t.Errorf("theme injected %d times, want once", n)
	}
}

// Only the head says whether a page is already themed. A body that happens to
// mention the theme's id, say a Dag doc or a log line quoting it, is still an
// unthemed page.
func TestInjectAirflowThemeLooksOnlyAtTheHeadForAnEarlierTheme(t *testing.T) {
	page := `<html><head></head><body><pre>&lt;style id="astro-theme"&gt;</pre><code>id="astro-theme"</code></body></html>`
	resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	got := readBody(t, resp)
	if i := strings.Index(got, themeTag); i < 0 || i > strings.Index(got, "</head>") {
		t.Error("a mention of the theme's id in the body kept the theme out of the head")
	}
}

// The theme is the proxy's own, on every page it relays, and a host's hooks see
// the themed page after it.
func TestTheProxyThemesEveryAirflowPage(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><head></head><body>airflow</body></html>")
	}))
	t.Cleanup(backend.Close)
	_, port, _ := strings.Cut(strings.TrimPrefix(backend.URL, "http://"), ":")

	p := NewProxy("0", NewStore(t.TempDir()))
	var hookSawTheme bool
	p.ModifyResponse = []func(*http.Response) error{func(resp *http.Response) error {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		hookSawTheme = bytes.Contains(body, []byte(themeTag))
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}}

	rec := httptest.NewRecorder()
	p.getOrCreateProxy(port).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x.localhost/", http.NoBody))

	if !strings.Contains(rec.Body.String(), themeTag) {
		t.Errorf("relayed page is not themed: %q", rec.Body.String())
	}
	if !hookSawTheme {
		t.Error("the host's hook ran before the theme, so it cannot build on the themed page")
	}
}

// The snippet goes exactly before the marker whatever bytes come first. Lower-
// casing the body to search it changes its length when it holds invalid UTF-8
// (a Latin-1 é becomes a three-byte replacement character) or a rune whose
// lower case is a different size (the Kelvin sign becomes a one-byte k), and an
// index found in the lowered copy then lands somewhere else in the original.
func TestInjectIntoHTMLPlacesTheSnippetAfterBytesThatChangeSizeWhenLowered(t *testing.T) {
	for name, head := range map[string]string{
		"invalid UTF-8": "<title>caf\xe9 \xe9t\xe9 \xe9</title>",
		"Kelvin sign":   "<title>KKKK</title>",
	} {
		t.Run(name, func(t *testing.T) {
			page := "<html><head>" + head + "</head><body>airflow</body></html>"
			resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
			if err := InjectIntoHTML(resp, "<style>x</style>", "</head>"); err != nil {
				t.Fatal(err)
			}
			want := "<html><head>" + head + "<style>x</style></head><body>airflow</body></html>"
			if got := readBody(t, resp); got != want {
				t.Errorf("snippet misplaced:\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// Strings of different lengths never match, including one that is a prefix or
// an extension of the other, which would otherwise read past the shorter.
func TestEqualFoldASCIIComparesLengths(t *testing.T) {
	for _, b := range []string{"</hea", "</head>x", "</HEAD>>"} {
		if equalFoldASCII([]byte(b), "</head>") {
			t.Errorf("%q matched </head>", b)
		}
	}
	if !equalFoldASCII([]byte("</HeAd>"), "</head>") {
		t.Error("</HeAd> did not match </head> ignoring case")
	}
}

// A marker of any length works, not only ones the size of </head>.
func TestInjectIntoHTMLTakesAMarkerOfAnyLength(t *testing.T) {
	for _, marker := range []string{"</p>", "</noscript>", "</header>"} {
		page := "<html><head></head><body>" + marker + "</body></html>"
		resp := newHTMLResponse(t, page, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
		if err := InjectIntoHTML(resp, "<i>x</i>", marker); err != nil {
			t.Fatal(err)
		}
		if got := readBody(t, resp); !strings.Contains(got, "<i>x</i>"+marker) {
			t.Errorf("snippet not before %s: %q", marker, got)
		}
	}
}

// A HEAD response carries the headers of the page it describes and no body.
// Reading it gives zero bytes, and rewriting Content-Length from that tells the
// client the page is empty.
func TestInjectIntoHTMLLeavesAHeadResponseAlone(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{},
		Body:          http.NoBody,
		ContentLength: 5000,
		Request:       httptest.NewRequest(http.MethodHead, "http://x.localhost/", http.NoBody),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Length", "5000")

	if err := InjectAirflowTheme(resp); err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Content-Length"); got != "5000" {
		t.Errorf("Content-Length = %q, want the page's own 5000", got)
	}
	if resp.ContentLength != 5000 {
		t.Errorf("ContentLength = %d, want 5000", resp.ContentLength)
	}
}

// A status that carries no body is passed through as it came.
func TestInjectIntoHTMLLeavesABodylessStatusAlone(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		resp := newHTMLResponse(t, "<html><head></head></html>", false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
		resp.StatusCode = status
		if err := InjectAirflowTheme(resp); err != nil {
			t.Fatal(err)
		}
		if got := readBody(t, resp); strings.Contains(got, themeTag) {
			t.Errorf("status %d was rewritten", status)
		}
	}
}

// An encoding the injector can't read is left exactly as it came, body and
// header both. Stripping Content-Encoding from a brotli body tells the browser
// to render compressed bytes as text.
func TestInjectIntoHTMLLeavesAnEncodingItCannotReadAlone(t *testing.T) {
	for _, enc := range []string{"br", "deflate", "zstd"} {
		const raw = "\x8b\x0b\x80<html><head></head></html>\x03"
		resp := newHTMLResponse(t, raw, false) //nolint:bodyclose // newHTMLResponse registers t.Cleanup
		resp.Header.Set("Content-Encoding", enc)
		if err := InjectAirflowTheme(resp); err != nil {
			t.Fatal(err)
		}
		if got := resp.Header.Get("Content-Encoding"); got != enc {
			t.Errorf("Content-Encoding = %q, want %q kept", got, enc)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != raw {
			t.Errorf("%s body was rewritten", enc)
		}
	}
}
