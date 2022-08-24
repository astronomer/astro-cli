package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func Test_isCalledFromUnitTestFile(t *testing.T) {
	if got := isCalledFromUnitTestFile(); got != true {
		t.Errorf("isCalledFromUnitTestFile() = %v, want %v", got, true)
	}
}

func TestSetVersion(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "set version",
			args: args{v: "0.29.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetVersion(tt.args.v)
			assert.Equal(t, version, tt.args.v)
		})
	}
}

func TestSanitiseVersionString(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic case",
			args: args{v: "0.29.0"},
			want: "v0.29.0",
		},
		{
			name: "version with pre-release",
			args: args{v: "0.29.0-rc2"},
			want: "v0.29.0",
		},
		{
			name: "version with v as prefix",
			args: args{v: "v0.29.2"},
			want: "v0.29.2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitiseVersionString(tt.args.v); got != tt.want {
				t.Errorf("sanitiseVersionString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCallerFunctionName(t *testing.T) {
	var res string
	testFunc := func() {
		res = getCallerFunctionName()
	}
	testFunc()
	assert.Equal(t, res, "TestGetCallerFunctionName")
}

func TestVerifyVersionMatch(t *testing.T) {
	type args struct {
		version         string
		funcRestriction VersionRestrictions
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "basic case",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{GTE: "0.29.0"}},
			want: true,
		},
		{
			name: "equal case",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: true,
		},
		{
			name: "equal case without version",
			args: args{version: "0.31.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: false,
		},
		{
			name: "equal case with v prefix",
			args: args{version: "v0.30.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: true,
		},
		{
			name: "case with gte and lt",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: true,
		},
		{
			name: "case with version outside gte and lt",
			args: args{version: "0.28.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: false,
		},
		{
			name: "case with version outside gte and lt",
			args: args{version: "0.31.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: false,
		},
		{
			name: "case with lt",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{LT: "0.31.0"}},
			want: true,
		},
		{
			name: "case with no restriction",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyVersionMatch(tt.args.version, tt.args.funcRestriction); got != tt.want {
				t.Errorf("VerifyVersionMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateAvailability(t *testing.T) {
	testUtil.InitTestConfig("software")

	mockResponse := Response{
		Data: ResponseData{
			GetAppConfig: &AppConfig{Version: "0.30.0"},
		},
	}
	jsonResponse, jsonErr := json.Marshal(mockResponse)
	assert.NoError(t, jsonErr)

	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})
	api := ClientImplementation{client: newInternalClient(client)}

	t.Run("basic case", func(t *testing.T) {
		err := api.ValidateAvailability()
		assert.NoError(t, err)
	})

	t.Run("basic case bypassing test file check", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		err := api.ValidateAvailability()
		assert.NoError(t, err)
	})

	t.Run("basic case with method restriction", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		houstonMethodAvailabilityByVersion["TestValidateAvailability"] = VersionRestrictions{GTE: "0.29.0"}
		err := api.ValidateAvailability()
		assert.NoError(t, err)
	})

	t.Run("negative case with method restriction", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"

		pc, _, _, _ := runtime.Caller(0)
		fName := runtime.FuncForPC(pc).Name()
		methodName := fName[strings.LastIndex(fName, ".")+1:] // split by . and get the last part of it
		methodName, _, _ = strings.Cut(methodName, "-")

		houstonMethodAvailabilityByVersion[methodName] = VersionRestrictions{GTE: "0.29.0"}
		err := api.ValidateAvailability()
		assert.ErrorIs(t, err, ErrMethodNotImplemented{MethodName: methodName})
	})
}
