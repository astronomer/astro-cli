package houston

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"golang.org/x/mod/semver"
)

var version string

func Call[fReq any, fResp any, fType func(any) (any, error)](houstonFunc func(fReq) (fResp, error), req fReq) (fResp, error) {
	if version == "" {
		version = GetHoustonVersion()
	}
	// get functionName from houstonFunc
	funcName := GetFunctionName(houstonFunc)
	if verifyMethodVersionMatch(version, funcName) {
		return houstonFunc(req)
	}
	var res fResp
	return res, errors.New("method not available")
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// type FType[P []any, S any] []any

// func (f FType[P, S]) Decorate(version string, fc func(P) (S, error)) (S, error) {
// 	if verifyMethodVersionMatch(version) {
// 		return fc([]any{f[0]})
// 	}
// 	var res S
// 	return res, errors.New("method not implemented")
// }

func verifyMethodVersionMatch(version, funcName string) bool {
	versionRestriction, ok := houstonMethodAvailabilityByVersion[funcName]
	if !ok {
		return true
	}
	if len(versionRestriction.eq) > 0 {
		for _, versionAvailable := range versionRestriction.eq {
			if versionAvailable == version {
				return true
			}
		}
		return false
	}
	if versionRestriction.gte != "" && versionRestriction.lt != "" {
		if semver.Compare(version, versionRestriction.gte) >= 0 && semver.Compare(version, versionRestriction.lt) <= 0 {
			return true
		}
		return false
	}
	if versionRestriction.gte != "" {
		if semver.Compare(version, versionRestriction.gte) > 0 {
			return true
		}
		return false
	}
	if versionRestriction.lt != "" {
		if semver.Compare(version, versionRestriction.lt) < 0 {
			return true
		}
		return false
	}
	return true
}

type methodVersionRestrictions struct {
	gte string
	lt  string
	eq  []string
}

var houstonMethodAvailabilityByVersion = map[string]methodVersionRestrictions{
	"UpdateDeploymentImage": {gte: "0.29.2"},

	"UpdateDeploymentRuntime":     {gte: "0.29.0"},
	"GetRuntimeReleases":          {gte: "0.29.0"},
	"GetTeam":                     {gte: "0.29.0"},
	"GetTeamUsers":                {gte: "0.29.0"},
	"ListTeams":                   {gte: "0.29.0"},
	"CreateTeamSystemRoleBinding": {gte: "0.29.0"},
	"DeleteTeamSystemRoleBinding": {gte: "0.29.0"},
	"AddWorkspaceTeam":            {gte: "0.29.0"},
	"DeleteWorkspaceTeam":         {gte: "0.29.0"},
	"ListWorkspaceTeamsAndRoles":  {gte: "0.29.0"},
	"UpdateWorkspaceTeamRole":     {gte: "0.29.0"},
	"GetWorkspaceTeamRole":        {gte: "0.29.0"},
}

func GetHoustonVersion() string {
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
	client := newInternalClient(httpClient)

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
