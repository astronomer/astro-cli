package organization

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

var (
	errMock      = errors.New("mock error")
	mockResponse = []OrgRes{
		{
			AuthServiceID: "auth-service-id",
			Name:          "name",
		},
		{
			AuthServiceID: "auth-service-id-2",
			Name:          "name-2",
		},
	}
)

func TestList(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("organization list success", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		buf := new(bytes.Buffer)
		err := List(buf)
		assert.NoError(t, err)
	})

	t.Run("organization list error", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{}
		})

		buf := new(bytes.Buffer)
		err := List(buf)
		assert.Contains(t, err.Error(), "could not retrieve organization list:")
	})
}

func TestGetOrganizationSelection(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("get organiation selection success", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf)
		assert.NoError(t, err)
	})

	t.Run("get organization selection list error", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{}
		})

		buf := new(bytes.Buffer)
		_, err := getOrganizationSelection(buf)
		assert.Contains(t, err.Error(), "could not retrieve organization list:")
	})

	t.Run("get organization selection select error", func(t *testing.T) {
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf)
		assert.ErrorIs(t, err, errInvalidOrganizationKey)
	})
}

func TestSwitch(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	t.Run("successful switch with name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		AuthLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("name", mockClient, buf)
		assert.NoError(t, err)
	})

	t.Run("successful switch without name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		AuthLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, buf)
		assert.NoError(t, err)
	})

	t.Run("failed switch wrong name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		AuthLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("name-wrong", mockClient, buf)
		assert.ErrorIs(t, err, errInvalidOrganizationName)
	})

	t.Run("failed switch bad selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		AuthLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, buf)
		assert.ErrorIs(t, err, errInvalidOrganizationKey)
	})

	t.Run("failed switch bad login", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		AuthLogin = func(domain, id, token string, client astro.Client, out io.Writer, shouldDisplayLoginLink bool) error {
			return errMock
		}
		buf := new(bytes.Buffer)
		err := Switch("name", mockClient, buf)
		assert.ErrorIs(t, err, errMock)
	})
}
