package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// diagnosingLocator points every link at url and answers WhyUnavailable with
// why, counting how often it was asked.
type diagnosingLocator struct {
	url   string
	why   error
	asked *atomic.Int32
}

func (l diagnosingLocator) BaseURL(context.Context, instances.Instance) (string, error) {
	return l.url, nil
}

func (l diagnosingLocator) WhyUnavailable(context.Context, instances.Instance) error {
	l.asked.Add(1)
	return l.why
}

// airflowAnswering is an Airflow whose every endpoint answers status, the way
// the ingress in front of a hibernating Deployment answers 503.
func airflowAnswering(t *testing.T, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "no healthy upstream", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"3.1.0","dags":[],"total_entries":0}`))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func outageDeps(t *testing.T, status int, why error) (d Deps, stdout *bytes.Buffer, asked *atomic.Int32) {
	t.Helper()
	dir := instanceProject(t, cloudManifest)
	d, stdout, _ = instanceDeps(t, dir)
	d.Session = func(context.Context, string) (string, error) { return "Bearer session-token", nil }
	asked = &atomic.Int32{}
	d.Locator = anyDomain(diagnosingLocator{url: airflowAnswering(t, status), why: why, asked: asked})
	return d, stdout, asked
}

func hibernating() error {
	return &instancelocate.UnavailableError{
		Name:         "prod",
		DeploymentID: "clm2xk9dq000108l7a2b3c4d5",
		State:        instancelocate.ErrDeploymentHibernating,
	}
}

func TestAHibernatingDeploymentSaysSoAndHowToWakeIt(t *testing.T) {
	for _, args := range [][]string{
		{afName, "health", "-d", "prod"},
		{afName, "dags", "list", "-d", "prod"},
	} {
		t.Run(strings.Join(args[:len(args)-2], " "), func(t *testing.T) {
			d, _, asked := outageDeps(t, http.StatusServiceUnavailable, hibernating())
			err := execute(t, d, args...)
			if err == nil {
				t.Fatal("a hibernating Deployment answered")
			}
			if !strings.Contains(err.Error(), "is hibernating") ||
				!strings.Contains(err.Error(), "astro deployment wake-up clm2xk9dq000108l7a2b3c4d5") {
				t.Errorf("err = %q, want the hibernation and the wake-up command named", err)
			}
			if strings.Contains(err.Error(), "api generation") {
				t.Errorf("err = %q still leads with the failed generation probe", err)
			}
			var outage *outageError
			if !errors.As(err, &outage) || !strings.Contains(outage.cause.Error(), "503 Service Unavailable") {
				t.Errorf("the Airflow's own 503 is not kept as the cause: %v", err)
			}
			if n := asked.Load(); n != 1 {
				t.Errorf("Astro was asked %d times, want once", n)
			}
		})
	}
}

func TestAHibernatingDeploymentPublishesItsKind(t *testing.T) {
	d, stdout, _ := outageDeps(t, http.StatusServiceUnavailable, hibernating())
	if err := execute(t, d, afName, "dags", "list", "-d", "prod", "--output", "json"); err == nil {
		t.Fatal("a hibernating Deployment answered")
	}
	var published jsonError
	if err := json.Unmarshal(stdout.Bytes(), &published); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if published.Kind != KindDeploymentHibernating {
		t.Errorf("kind = %q, want %q", published.Kind, KindDeploymentHibernating)
	}
}

func TestAnUnexplainedOutageKeepsTheAirflowsError(t *testing.T) {
	d, _, asked := outageDeps(t, http.StatusServiceUnavailable, nil)
	err := execute(t, d, afName, "dags", "list", "-d", "prod")
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want the Airflow's own 503", err)
	}
	if asked.Load() != 1 {
		t.Errorf("Astro was asked %d times, want once", asked.Load())
	}
}

func TestAHealthyDeploymentsOutageShowsTheAirflowsErrorToo(t *testing.T) {
	why := &instancelocate.UnavailableError{
		Name:         "prod",
		DeploymentID: "clm2xk9dq000108l7a2b3c4d5",
		State:        instancelocate.ErrAirflowUnavailable,
	}
	d, _, _ := outageDeps(t, http.StatusServiceUnavailable, why)
	err := execute(t, d, afName, "dags", "list", "-d", "prod")
	if err == nil || !strings.Contains(err.Error(), "not answering yet") || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want Astro's explanation and the Airflow's own 503", err)
	}
}

func TestAstroIsNotAskedUnlessTheAirflowWasMissing(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		d, _, asked := outageDeps(t, status, hibernating())
		_ = execute(t, d, afName, "dags", "list", "-d", "prod")
		if n := asked.Load(); n != 0 {
			t.Errorf("status %d: Astro was asked %d times, want never", status, n)
		}
	}
}

type statusTransport []int

func (s *statusTransport) Do(context.Context, airflowapi.Request) (airflowapi.Response, error) {
	status := (*s)[0]
	*s = (*s)[1:]
	return airflowapi.Response{StatusCode: status}, nil
}

func TestALaterRealAnswerClearsTheOutage(t *testing.T) {
	w := &outageWatch{next: &statusTransport{http.StatusBadGateway, http.StatusNotFound}}
	for range 2 {
		if _, err := w.Do(context.Background(), airflowapi.Request{}); err != nil {
			t.Fatal(err)
		}
	}
	if w.missing.Load() {
		t.Error("a 404 after a 502 still reads as no Airflow behind the link")
	}
}
