package houston

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"golang.org/x/mod/semver"
)

var (
	Version string

	HoustonVersionQuery = `
	query AppConfig {
		appConfig {
			version
		}
	}`
)

func Call[fReq any, fResp any, fType func(any) (any, error)](houstonFunc func(fReq) (fResp, error), req fReq) (fResp, error) {
	if Version == "" { //fallback
		Version = GetHoustonVersion(nil)
	}
	// get functionName from houstonFunc
	funcName := GetFunctionName(houstonFunc)
	funcRestriction, ok := houstonMethodAvailabilityByVersion[funcName]
	if !ok {
		return houstonFunc(req)
	}
	if VerifyVersionMatch(Version, funcRestriction) {
		return houstonFunc(req)
	}
	var res fResp
	return res, errors.New("method not available")
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func VerifyVersionMatch(version string, funcRestriction VersionRestrictions) bool {
	orgVersion := version
	version = sanitiseVersionString(version)

	if len(funcRestriction.EQ) > 0 {
		for _, versionAvailable := range funcRestriction.EQ {
			if orgVersion == versionAvailable || semver.Compare(version, sanitiseVersionString(versionAvailable)) == 0 {
				return true
			}
		}
		return false
	}

	if funcRestriction.GTE != "" && funcRestriction.LT != "" {
		funcRestriction.GTE = sanitiseVersionString(funcRestriction.GTE)
		funcRestriction.LT = sanitiseVersionString(funcRestriction.LT)

		if semver.Compare(version, funcRestriction.GTE) >= 0 && semver.Compare(version, funcRestriction.LT) <= 0 {
			return true
		}
		return false
	}
	if funcRestriction.GTE != "" {
		funcRestriction.GTE = sanitiseVersionString(funcRestriction.GTE)

		return semver.Compare(version, funcRestriction.GTE) >= 0
	}
	if funcRestriction.LT != "" {
		funcRestriction.LT = sanitiseVersionString(funcRestriction.LT)

		return semver.Compare(version, funcRestriction.LT) < 0
	}
	return true
}

func sanitiseVersionString(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	preRelease := semver.Prerelease(v)
	if preRelease != "" {
		v = strings.TrimSuffix(v, preRelease)
	}
	return v
}

type VersionRestrictions struct {
	GTE string
	LT  string
	EQ  []string
}

var houstonMethodAvailabilityByVersion = map[string]VersionRestrictions{
	"UpdateDeploymentImage": {GTE: "0.29.2"},

	"UpdateDeploymentRuntime":     {GTE: "0.29.0"},
	"GetRuntimeReleases":          {GTE: "0.29.0"},
	"GetTeam":                     {GTE: "0.29.0"},
	"GetTeamUsers":                {GTE: "0.29.0"},
	"ListTeams":                   {GTE: "0.29.0"},
	"CreateTeamSystemRoleBinding": {GTE: "0.29.0"},
	"DeleteTeamSystemRoleBinding": {GTE: "0.29.0"},
	"AddWorkspaceTeam":            {GTE: "0.29.0"},
	"DeleteWorkspaceTeam":         {GTE: "0.29.0"},
	"ListWorkspaceTeamsAndRoles":  {GTE: "0.29.0"},
	"UpdateWorkspaceTeamRole":     {GTE: "0.29.0"},
	"GetWorkspaceTeamRole":        {GTE: "0.29.0"},
}

func GetHoustonVersion(client *Client) string {
	if client == nil {
		httpClient := httputil.NewHTTPClient()
		// configure http transport
		dialTimeout := config.CFG.HoustonDialTimeout.GetInt()
		// #nosec
		httpClient.HTTPClient.Transport = &http.Transport{
			Dial: (&net.Dialer{
				Timeout: time.Duration(dialTimeout) * time.Second,
			}).Dial,
			TLSHandshakeTimeout: time.Duration(dialTimeout) * time.Second,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: config.CFG.HoustonSkipVerifyTLS.GetBool()},
		}
		client = newInternalClient(httpClient)
	}

	req := Request{
		Query: HoustonVersionQuery,
	}

	var r *Response
	r, appConfigErr = req.DoWithClient(client)
	if appConfigErr != nil {
		appConfigErr = handleAPIErr(appConfigErr)
		return ""
	}
	return r.Data.GetAppConfig.Version
}
