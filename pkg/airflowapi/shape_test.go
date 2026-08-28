package airflowapi

import "testing"

func TestOddErrorDetailShape(t *testing.T) {
	got := logLine([]byte(`{"event":"Task failed with exception","error_detail":{"exc_type":"KeyError"}}`))
	t.Logf("got: %s", got)
}
