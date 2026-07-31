package instances

import (
	"context"
	"strings"
	"testing"
)

// TestAccountFromADCReadsEveryShapeItCan: the over-long-service-account advice
// is only worth printing when the account is identifiable, so what counts as
// identifiable is the thing to pin.
func TestAccountFromADCReadsEveryShapeItCan(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			"service account key",
			`{"type":"service_account","client_email":"orders@acme-data.iam.gserviceaccount.com"}`,
			"orders@acme-data.iam.gserviceaccount.com",
		},
		{
			// Workload identity federation and `gcloud --impersonate-service-account`
			// both write this shape, and neither carries a client_email.
			"impersonated external account",
			`{"type":"external_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/orders@acme-data.iam.gserviceaccount.com:generateAccessToken"}`,
			"orders@acme-data.iam.gserviceaccount.com",
		},
		{
			// A user login. There is no account and no length to worry about.
			"authorized user",
			`{"type":"authorized_user","client_id":"32555940559.apps.googleusercontent.com"}`,
			"",
		},
		{"not json", `{`, ""},
		{"empty", ``, ""},
		{
			"impersonation url in an unexpected shape",
			`{"service_account_impersonation_url":"https://example.invalid/nothing"}`,
			"",
		},
	}
	for _, tc := range cases {
		if got := accountFromADC([]byte(tc.json)); got != tc.want {
			t.Errorf("%s: account = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestGoogleRefreshHookNamesTheLongAccount: an Airflow 403 arrives as a status
// with nothing in it about why. The refresh hook is where the status and the
// identity behind it are both in hand, so it is where the fix gets named.
func TestGoogleRefreshHookNamesTheLongAccount(t *testing.T) {
	long := strings.Repeat("o", 45) + "@acme-data.iam.gserviceaccount.com"
	_, refresh := googleCredentials(Deps{
		GoogleToken:   func(context.Context) (string, error) { return "ya29.token", nil },
		GoogleAccount: func(context.Context) string { return long },
	})
	err := refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "numeric account id") {
		t.Fatalf("err = %v, want the pre-registration fix", err)
	}

	// A short account, and a user login: nothing to add, so the retry goes
	// ahead and Airflow's own refusal stands.
	for _, fine := range []string{"short@acme-data.iam.gserviceaccount.com", ""} {
		_, refresh := googleCredentials(Deps{
			GoogleToken:   func(context.Context) (string, error) { return "ya29.token", nil },
			GoogleAccount: func(context.Context) string { return fine },
		})
		if err := refresh(context.Background()); err != nil {
			t.Errorf("%q: refresh = %v, want it to let the retry through", fine, err)
		}
	}
}

func TestGoogleAccountAdviceOnlyFiresOnALongAccount(t *testing.T) {
	short := "orders@acme-data.iam.gserviceaccount.com"
	if advice := GoogleAccountAdvice(short); advice != "" {
		t.Errorf("a %d-character account got advice: %s", len(short), advice)
	}
	if advice := GoogleAccountAdvice(""); advice != "" {
		t.Errorf("a user login got advice: %s", advice)
	}
	long := strings.Repeat("o", 40) + "@acme-data.iam.gserviceaccount.com"
	advice := GoogleAccountAdvice(long)
	for _, want := range []string{long, "numeric account id", "pre-register"} {
		if !strings.Contains(advice, want) {
			t.Errorf("advice does not name %s: %s", want, advice)
		}
	}
}
