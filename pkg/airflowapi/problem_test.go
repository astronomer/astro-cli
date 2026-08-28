package airflowapi

import (
	"net/http"
	"testing"
)

// Airflow answers a refusal with a problem document, and some of them explain
// nothing: a null detail, and a title that repeats the status. Dumping that raw
// put five lines of JSON and half a docs URL into a health report where one line
// belongs. A body that is not a problem document still shows, because there it
// is everything the reader has.
func TestStatusErrorRendersProblemDocuments(t *testing.T) {
	const forbidden = `{
  "detail": null,
  "status": 403,
  "title": "Forbidden",
  "type": "http://apache-airflow-docs.s3-website.eu-central-1.amazonaws.com/docs/apache-airflow/stable/stable-rest-api-ref.html#section/Errors/PermissionDenied"
}`
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{{
		name:   "a title that only repeats the status adds nothing",
		status: http.StatusForbidden,
		body:   forbidden,
		want:   "GET /dagWarnings: airflow returned 403 Forbidden",
	}, {
		name:   "a detail is the best thing to show",
		status: http.StatusBadRequest,
		body:   `{"detail":"dag_ids is required","status":400,"title":"Bad Request"}`,
		want:   "GET /dagWarnings: airflow returned 400 Bad Request: dag_ids is required",
	}, {
		name:   "a title that says something new is kept",
		status: http.StatusForbidden,
		body:   `{"detail":null,"status":403,"title":"DAG access denied"}`,
		want:   "GET /dagWarnings: airflow returned 403 Forbidden: DAG access denied",
	}, {
		name:   "a body that is not a problem document still shows",
		status: http.StatusBadGateway,
		body:   "<html><body>502 Bad Gateway</body></html>",
		want:   "GET /dagWarnings: airflow returned 502 Bad Gateway: <html><body>502 Bad Gateway</body></html>",
	}, {
		name:   "an empty body leaves the status to speak",
		status: http.StatusForbidden,
		body:   "",
		want:   "GET /dagWarnings: airflow returned 403 Forbidden",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &StatusError{
				Method:     http.MethodGet,
				Path:       "/dagWarnings",
				StatusCode: tc.status,
				Body:       []byte(tc.body),
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("Error() =\n  %s\nwant\n  %s", got, tc.want)
			}
		})
	}
}
