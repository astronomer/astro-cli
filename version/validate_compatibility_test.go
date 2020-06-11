package version

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestValidateCompatibilityVersionsMatched(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.15.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	// check that there is no output because version matched
	assert.Equal(t, output, &bytes.Buffer{})
}

func TestValidateCompatibilityVersionsCliDowngrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "0.15.1",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.NoError(t, err)
	expected := "Your Astro CLI Version (0.17.1) is ahead of the server version (0.15.1). Consider downgrading your Astro CLI to match. See https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	// check that user can see correct message
	assert.Equal(t, expected, output.String())
}

func TestValidateCompatibilityVersionsCliUpgrade(t *testing.T) {
	testUtil.InitTestConfig()
	okResponse := `{
		"data": {
			"appConfig": {
				"version": "1.0.0",
				"baseDomain": "local.astronomer.io",
				"smtpConfigured": true,
				"manualReleaseNames": false
			}
		}
	}`
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(okResponse)),
			Header:     make(http.Header),
		}
	})
	api := houston.NewHoustonClient(client)
	output := new(bytes.Buffer)
	cliVer := "0.17.1"
	err := ValidateCompatibility(api, output, cliVer, false)
	assert.Error(t, err)
	expected := "There is an update for Astro CLI. You're using version 0.17.1, but 1.0.0 is the server version. Please upgrade to the matching version before continuing. See https://www.astronomer.io/docs/cli-quickstart for more information.\n"
	// check that user can see correct message
	assert.EqualError(t, err, expected)

	output = new(bytes.Buffer)
	err = ValidateCompatibility(api, output, cliVer, true)
	assert.NoError(t, err)
	expected = ""
	// check that user can bypass major version check
	assert.Equal(t, expected, output.String())
}

func TestIsBehindMajor(t *testing.T) {
	cliVer := "0.17.1"
	serverVer := "0.18.0"
	assert.False(t, isBehindMajor(serverVer, cliVer))

	cliVer = "1.0.0"
	serverVer = "1.1.0"
	assert.False(t, isBehindMajor(serverVer, cliVer))

	cliVer = "1.0.0"
	serverVer = "2.0.0"
	assert.True(t, isBehindMajor(serverVer, cliVer))
}

func TestIsBehindMinor(t *testing.T) {
	cliVer := "0.17.1"
	serverVer := "0.17.8"
	assert.False(t, isBehindMinor(serverVer, cliVer))

	cliVer = "1.1.0"
	serverVer = "1.2.0"
	assert.True(t, isBehindMinor(serverVer, cliVer))

	cliVer = "1.0.0"
	serverVer = "2.0.0"
	assert.True(t, isBehindMinor(serverVer, cliVer))
}

func TestIsBehindPatch(t *testing.T) {
	cliVer := "0.17.0"
	serverVer := "0.17.1"
	assert.True(t, isBehindPatch(serverVer, cliVer))
}

func TestIsAheadMinor(t *testing.T) {
	cliVer := "0.18.0"
	serverVer := "0.17.0"
	assert.True(t, isAheadMinor(serverVer, cliVer))
}

func TestFormatMajor(t *testing.T) {
	exp := "1"
	act := formatMajor("1.1.0")

	assert.Equal(t, exp, act)
}

func TestFormatMinor(t *testing.T) {
	exp := "0.17"
	act := formatMinor("0.17.0")

	assert.Equal(t, exp, act)
}

func TestFormatLtConstraint(t *testing.T) {
	exp := "< 0.17.0"
	act := formatLtConstraint("0.17.0")

	assert.Equal(t, exp, act)
}

func TestFormatGtConstraint(t *testing.T) {
	exp := "> 0.17.0"
	act := formatGtConstraint("0.17.0")

	assert.Equal(t, exp, act)
}

func TestCheckFormattedConstraint(t *testing.T) {
	cliBehind := checkFormattedConstraint("< 1.1.2", "1.1.1")
	cliAhead := checkFormattedConstraint("> 1.1.1", "1.1.2")
	cliMinorBehind := checkFormattedConstraint("< 1.1", "1.0")
	cliMinorAhead := checkFormattedConstraint("> 1.1", "1.2")
	cliMajorBehind := checkFormattedConstraint("< 1", "0")
	cliMajorAhead := checkFormattedConstraint("> 1", "2")

	assert.True(t, cliBehind)
	assert.True(t, cliAhead)
	assert.True(t, cliMinorBehind)
	assert.True(t, cliMinorAhead)
	assert.True(t, cliMajorBehind)
	assert.True(t, cliMajorAhead)
}

func TestGetConstraint(t *testing.T) {
	ver, err := parseVersion("0.17.1")
	assert.NoError(t, err)

	majGt := getConstraint("> 0.18.0")
	majLt := getConstraint("> 0.16.0")
	patchLt := getConstraint("< 0.17")
	patchGt := getConstraint("< 0.18")

	if assert.NotNil(t, majGt) {
		assert.False(t, majGt.Check(ver))
	}

	if assert.NotNil(t, majLt) {
		assert.True(t, majLt.Check(ver))
	}

	if assert.NotNil(t, patchLt) {
		assert.False(t, patchLt.Check(ver))
	}

	if assert.NotNil(t, patchGt) {
		assert.True(t, patchGt.Check(ver))
	}
}

func TestGetVersion(t *testing.T) {
	ver, err := parseVersion("0.17.1")
	assert.NoError(t, err)
	if assert.NotNil(t, ver) {
		assert.Equal(t, uint64(0), ver.Major())
		assert.Equal(t, uint64(17), ver.Minor())
		assert.Equal(t, uint64(1), ver.Patch())
	}
}
