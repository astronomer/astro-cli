package cmd

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/auth"
	_ "github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtils "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/user"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
	TestEmail     string
	TestPassword  string
	TestWorkspace string
	TestDomain    string
	Temp          string
	Client        houston.ClientInterface
	Fs            afero.Fs
}

func (ts *IntegrationTestSuite) SetupSuite() {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig("docker")
	afero.WriteFile(fs, config.HomeConfigFile, configYaml, 0777)
	config.InitConfig(fs)
	rand.Seed(time.Now().UnixNano())
	ts.Fs = fs
	ts.Client = houston.NewClient(httputil.NewHTTPClient())
	ts.TestDomain = testUtils.GetEnv("HOUSTON_HOST", "localhost")
	ts.TestEmail = fmt.Sprintf("test%d@astronomer.io", rand.Intn(100))
	ts.TestPassword = "pass"
	ts.TestWorkspace = "test-workspace"
	user.Create(ts.TestEmail, ts.TestPassword, ts.Client, new(bytes.Buffer))
}

func (ts *IntegrationTestSuite) Test2CreateWorkspace() {
	output := new(bytes.Buffer)
	err := auth.Login(ts.TestDomain, true, ts.TestEmail, ts.TestPassword, ts.Client, output)
	assert.NoError(ts.T(), err)
	out, err := executeCommandC(ts.Client, "workspace", "create", ts.TestWorkspace)
	assert.NoError(ts.T(), err)
	expectedOut := "Successfully created workspace"
	assert.Contains(ts.T(), out, expectedOut)
}

func TestCreateWorkspaceSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
