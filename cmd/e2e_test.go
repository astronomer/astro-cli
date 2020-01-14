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
	Client        *houston.Client
	Fs            afero.Fs
}

func (suite *IntegrationTestSuite) SetupSuite() {
	fs := afero.NewMemMapFs()
	configYaml := testUtils.NewTestConfig()
	afero.WriteFile(fs, config.HomeConfigFile, []byte(configYaml), 0777)
	config.InitConfig(fs)
	rand.Seed(time.Now().UnixNano())
	suite.Fs = fs
	suite.Client = houston.NewHoustonClient(httputil.NewHTTPClient())
	suite.TestDomain = testUtils.GetEnv("HOUSTON_HOST", "localhost")
	suite.TestEmail = fmt.Sprintf("test%d@astronomer.io", rand.Intn(100))
	suite.TestPassword = "pass"
	suite.TestWorkspace = "test-workspace"
	user.Create(suite.TestEmail, suite.TestPassword, suite.Client, new(bytes.Buffer))
}

func (suite *IntegrationTestSuite) Test2CreateWorkspace() {
	output := new(bytes.Buffer)
	err := auth.Login(suite.TestDomain, true, suite.TestEmail, suite.TestPassword, suite.Client, output)
	assert.NoError(suite.T(), err)
	_, out, err := executeCommandC(suite.Client, "workspace", "create", suite.TestWorkspace)
	assert.NoError(suite.T(), err)
	expectedOut := "Successfully created workspace"
	assert.Contains(suite.T(), out, expectedOut)
}

func TestCreateWorkspaceSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
