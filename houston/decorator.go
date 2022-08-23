package houston

import (
	"crypto/tls"
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
	version                string
	ApplyDecoratorForTests bool
)

const houstonVersionQuery = `query AppConfig {
								appConfig {
									version
								}
							}`

type VersionRestrictions struct {
	GTE string
	LT  string
	EQ  []string
}

var houstonMethodAvailabilityByVersion = map[string]VersionRestrictions{
	"GetTeam":                     {GTE: "0.30.0"},
	"GetTeamUsers":                {GTE: "0.30.0"},
	"ListTeams":                   {GTE: "0.30.0"},
	"CreateTeamSystemRoleBinding": {GTE: "0.30.0"},
	"DeleteTeamSystemRoleBinding": {GTE: "0.30.0"},
	"AddWorkspaceTeam":            {GTE: "0.30.0"},
	"DeleteWorkspaceTeam":         {GTE: "0.30.0"},
	"ListWorkspaceTeamsAndRoles":  {GTE: "0.30.0"},
	"UpdateWorkspaceTeamRole":     {GTE: "0.30.0"},
	"GetWorkspaceTeamRole":        {GTE: "0.30.0"},

	"UpdateDeploymentImage": {GTE: "0.29.2"},

	"UpdateDeploymentRuntime": {GTE: "0.29.0"},
	"GetRuntimeReleases":      {GTE: "0.29.0"},
}

func Call[fReq any, fResp any, fType func(any) (any, error)](houstonFunc func(fReq) (fResp, error), req fReq) (fResp, error) {
	if !ApplyDecoratorForTests && isCalledFromUnitTestFile() { //bypassing this decorator for unit tests
		return houstonFunc(req)
	}
	if version == "" { //fallback in case version is not set
		version = getHoustonVersion()
	}
	// get functionName from houstonFunc
	funcName := getFunctionName(houstonFunc)
	funcRestriction, ok := houstonMethodAvailabilityByVersion[funcName]
	if !ok {
		return houstonFunc(req)
	}
	if VerifyVersionMatch(version, funcRestriction) {
		return houstonFunc(req)
	}
	var res fResp
	return res, ErrMethodNotImplemented{MethodName: funcName}
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

		if semver.Compare(version, funcRestriction.GTE) >= 0 && semver.Compare(version, funcRestriction.LT) < 0 {
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

func getFunctionName(i interface{}) string {
	runtimeFuncName := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name() // naming convention: github.com/astronomer/astro-cli/houston.ClientInterface.GetTeam-fm
	methodName := runtimeFuncName[strings.LastIndex(runtimeFuncName, ".")+1:] // split by . and get the last part of it
	methodName, _, _ = strings.Cut(methodName, "-")                           //get the string before the first instance of hyphen
	return methodName
}

// prepare version string to be consumed by "golang.org/x/mod/semver" package
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

func getHoustonVersion() string {
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
		Query: houstonVersionQuery,
	}

	var r *Response
	r, appConfigErr = req.DoWithClient(client)
	if appConfigErr != nil {
		appConfigErr = handleAPIErr(appConfigErr)
		return ""
	}
	return r.Data.GetAppConfig.Version
}

func SetVersion(v string) {
	version = v
}

func isCalledFromUnitTestFile() bool {
	// depth is usually 4 because unit test method will call the actual testing method
	// which will internally call the decorator function and then `isCalledFromUnitTestFile` will be called
	for fileDepth := 1; fileDepth <= 10; fileDepth++ {
		_, file, _, ok := runtime.Caller(fileDepth)
		if ok && strings.HasSuffix(file, "_test.go") { // function is called from a unit test file
			return true
		}
	}
	return false
}
