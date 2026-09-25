package airflowrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPythonFallback(t *testing.T) {
	cases := []struct {
		name, requiresPython, airflow, want string
	}{
		// A stated requires-python is uv's to read; nothing is forced past it.
		{"stated, airflow 3", ">=3.10", "3.1", ""},
		{"stated, old airflow 2", ">=3.10,<3.12", "2.7", ""},
		{"stated wins over a pin the fallback would cap", ">=3.9", "2.8.4", ""},

		{"unset, airflow 3", "", "3.1", DefaultPython},
		{"unset, bare airflow 3", "", "3", DefaultPython},
		{"unset, airflow 2.9", "", "2.9", DefaultPython},
		{"unset, airflow 2.10.5", "", "2.10.5", DefaultPython},
		{"unset, bare airflow 2 is the newest 2", "", "2", DefaultPython},
		{"unset, airflow 2.8", "", "2.8", "3.11"},
		{"unset, airflow 2.7.3", "", "2.7.3", "3.11"},
		{"unset, no pin", "", "", DefaultPython},

		// Whitespace is not a statement.
		{"blank requires-python", "  ", "2.7", "3.11"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, PythonFallback(tc.requiresPython, tc.airflow))
		})
	}
}
