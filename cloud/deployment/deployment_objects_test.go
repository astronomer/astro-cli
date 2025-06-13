package deployment

import (
	"bytes"
	"errors"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	airflowclient_mocks "github.com/astronomer/astro-cli/airflow-client/mocks"
	"github.com/stretchr/testify/mock"
)

var (
	mockResp = airflowclient.Response{
		Connections: []airflowclient.Connection{
			{ConnID: "conn1", ConnType: "type1"},
			{ConnID: "conn2", ConnType: "type2"},
		},
	}
	testAirflowURL  = "http://airflow-url?orgID=orgId"
	testConnID      = "test-conn"
	testConnType    = "test-type"
	testDescription = "test-description"
	testHost        = "test-host"
	testLogin       = "test-login"
	testPassword    = "test-password"
	testSchema      = "test-schema"
	testExtra       = "test-extra"
	testPort        = 1234
	errTest         = errors.New("error")
	errUpdate       = errors.New("update error")
	errCreate       = errors.New("create error")
)

func (s *Suite) TestConnectionList() {
	s.Run("happy path TestConnectionList", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Once()
		err := ConnectionList(testAirflowURL, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when GetConnections returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetConnections", mock.AnythingOfType("string")).Return(mockResp, errTest).Once()
		err := ConnectionList(testAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

func (s *Suite) TestConnectionCreate() {
	s.Run("happy path TestConnectionCreate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateConnection", testAirflowURL, mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		err := ConnectionCreate(testAirflowURL, testConnID, testConnType, testDescription, testHost, testLogin, testPassword, testSchema, testExtra, testPort, mockClient, out)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("error path when CreateConnection returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateConnection", testAirflowURL, mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		err := ConnectionCreate(testAirflowURL, testConnID, testConnType, testDescription, testHost, testLogin, testPassword, testSchema, testExtra, testPort, mockClient, out)
		s.EqualError(err, "error")
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestConnectionUpdate() {
	s.Run("happy path TestConnectionUpdate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateConnection", testAirflowURL, mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		err := ConnectionUpdate(testAirflowURL, testConnID, testConnType, testDescription, testHost, testLogin, testPassword, testSchema, testExtra, testPort, mockClient, out)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("error path when UpdateConnection returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateConnection", testAirflowURL, mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		err := ConnectionUpdate(testAirflowURL, testConnID, testConnType, testDescription, testHost, testLogin, testPassword, testSchema, testExtra, testPort, mockClient, out)
		s.EqualError(err, "error")
		mockClient.AssertExpectations(s.T())
	})
}

var (
	fromAirflowURL = "from_airflow_url?orgID=orgId"
	toAirflowURL   = "to_airflow_url?orgID=orgId"

	fromConnections = []airflowclient.Connection{
		{ConnID: "conn1", ConnType: "type1", Description: "desc1"},
		{ConnID: "conn2", ConnType: "type2", Description: "desc2"},
	}
	toConnections = []airflowclient.Connection{
		{ConnID: "conn2", ConnType: "type2", Description: "desc2"},
		{ConnID: "conn3", ConnType: "type3", Description: "desc3"},
	}
)

func (s *Suite) TestCopyConnection() {
	s.Run("happy path TestCopyConnection", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetConnections", fromAirflowURL).Return(airflowclient.Response{Connections: fromConnections}, nil).Once()
		mockClient.On("GetConnections", toAirflowURL).Return(airflowclient.Response{Connections: toConnections}, nil).Once()

		// Mock UpdateConnection and CreateConnection for target deployment
		mockClient.On("UpdateConnection", toAirflowURL, &fromConnections[1]).Return(nil).Once()
		mockClient.On("CreateConnection", toAirflowURL, &fromConnections[0]).Return(nil).Once()

		err := CopyConnection(fromAirflowURL, toAirflowURL, mockClient, out)
		s.NoError(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("error path when GetConnections returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		// Mock GetConnections for source deployment
		mockClient.On("GetConnections", fromAirflowURL).Return(airflowclient.Response{Connections: fromConnections}, errTest).Once()

		err := CopyConnection(fromAirflowURL, toAirflowURL, mockClient, out)
		s.EqualError(err, "error")

		mockClient.AssertExpectations(s.T())
	})

	s.Run("error path when UpdateConnection returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetConnections", fromAirflowURL).Return(airflowclient.Response{Connections: fromConnections}, nil).Once()
		mockClient.On("GetConnections", toAirflowURL).Return(airflowclient.Response{Connections: toConnections}, nil).Once()

		// Mock UpdateConnection for target deployment
		mockClient.On("UpdateConnection", toAirflowURL, &fromConnections[1]).Return(errUpdate).Once()
		mockClient.On("CreateConnection", toAirflowURL, &fromConnections[0]).Return(nil).Once()

		err := CopyConnection(fromAirflowURL, toAirflowURL, mockClient, out)
		s.EqualError(err, "update error")

		mockClient.AssertExpectations(s.T())
	})

	s.Run("error path when UpdateConnection returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetConnections", fromAirflowURL).Return(airflowclient.Response{Connections: fromConnections}, nil).Once()
		mockClient.On("GetConnections", toAirflowURL).Return(airflowclient.Response{Connections: toConnections}, nil).Once()

		// Mock UpdateConnection for target deployment
		mockClient.On("CreateConnection", toAirflowURL, &fromConnections[0]).Return(errCreate).Once()

		err := CopyConnection(fromAirflowURL, toAirflowURL, mockClient, out)
		s.EqualError(err, "create error")

		mockClient.AssertExpectations(s.T())
	})
}

var mockVarResp = &airflowclient.Response{
	Variables: []airflowclient.Variable{
		{Key: "var1", Description: "desc1"},
		{Key: "var2", Description: "desc2"},
	},
}

func (s *Suite) TestAirflowVariableList() {
	s.Run("happy path TestAirflowVariableList", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetVariables", testAirflowURL).Return(*mockVarResp, nil).Once()

		err := AirflowVariableList(testAirflowURL, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when GetVariables returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetVariables", testAirflowURL).Return(*mockVarResp, errTest).Once()

		err := AirflowVariableList(testAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

func (s *Suite) TestVariableCreate() {
	s.Run("happy path TestVariableCreate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateVariable", testAirflowURL, mock.AnythingOfType("airflowclient.Variable")).Return(nil).Once()

		err := VariableCreate(testAirflowURL, "value1", "key1", "desc1", mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when CreateVariable returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateVariable", testAirflowURL, mock.AnythingOfType("airflowclient.Variable")).Return(errTest).Once()

		err := VariableCreate(testAirflowURL, "value2", "key2", "desc2", mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

func (s *Suite) TestVariableUpdate() {
	s.Run("happy path TestVariableUpdate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateVariable", testAirflowURL, mock.AnythingOfType("airflowclient.Variable")).Return(nil).Once()

		err := VariableUpdate(testAirflowURL, "new_value1", "key1", "desc1", mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when UpdateVariable returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateVariable", testAirflowURL, mock.AnythingOfType("airflowclient.Variable")).Return(errTest).Once()

		err := VariableUpdate(testAirflowURL, "new_value2", "key2", "desc2", mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

var (
	fromVaraibles = []airflowclient.Variable{
		{Key: "var1", Description: "desc1"},
		{Key: "var2", Description: "desc2"},
	}
	toVaraiables = []airflowclient.Variable{
		{Key: "var2", Description: "desc2"},
		{Key: "var3", Description: "desc3"},
	}
)

func (s *Suite) TestCopyVariable() {
	s.Run("happy path TestCopyVariable", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetVariables", fromAirflowURL).Return(airflowclient.Response{Variables: fromVaraibles}, nil).Once()
		mockClient.On("GetVariables", toAirflowURL).Return(airflowclient.Response{Variables: toVaraiables}, nil).Once()
		mockClient.On("UpdateVariable", toAirflowURL, fromVaraibles[1]).Return(nil).Once()
		mockClient.On("CreateVariable", toAirflowURL, fromVaraibles[0]).Return(nil).Once()

		err := CopyVariable(fromAirflowURL, toAirflowURL, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when GetVariables returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetVariables", fromAirflowURL).Return(airflowclient.Response{}, errTest).Once()

		err := CopyVariable(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})

	s.Run("error path when UpdateVariable returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetVariables", fromAirflowURL).Return(airflowclient.Response{Variables: fromVaraibles}, nil).Once()
		mockClient.On("GetVariables", toAirflowURL).Return(airflowclient.Response{Variables: toVaraiables}, nil).Once()
		mockClient.On("CreateVariable", toAirflowURL, fromVaraibles[0]).Return(nil).Once()
		mockClient.On("UpdateVariable", toAirflowURL, fromVaraibles[1]).Return(errTest).Once()

		err := CopyVariable(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})

	s.Run("error path when CreateVariable returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetVariables", fromAirflowURL).Return(airflowclient.Response{Variables: fromVaraibles}, nil).Once()
		mockClient.On("GetVariables", toAirflowURL).Return(airflowclient.Response{Variables: toVaraiables}, nil).Once()
		mockClient.On("CreateVariable", toAirflowURL, fromVaraibles[0]).Return(errTest).Once()

		err := CopyVariable(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

var mockPoolsResp = &airflowclient.Response{
	Pools: []airflowclient.Pool{
		{Name: "pool1", Slots: 5},
		{Name: "pool2", Slots: 10},
	},
}

func (s *Suite) TestPoolList() {
	s.Run("happy path TestPoolList", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetPools", testAirflowURL).Return(*mockPoolsResp, nil).Once()

		err := PoolList(testAirflowURL, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when GetPools returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetPools", testAirflowURL).Return(*mockPoolsResp, errTest).Once()

		err := PoolList(testAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

func (s *Suite) TestPoolCreate() {
	s.Run("happy path TestPoolCreate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		poolName := "test_pool"
		poolSlots := 5
		poolDescription := "Test pool for unit testing"
		poolIncludeDeferred := true
		mockClient.On("CreatePool", testAirflowURL, airflowclient.Pool{
			Name:            poolName,
			Slots:           poolSlots,
			Description:     poolDescription,
			IncludeDeferred: poolIncludeDeferred,
		}).Return(nil).Once()

		err := PoolCreate(testAirflowURL, poolName, poolDescription, poolSlots, poolIncludeDeferred, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when CreatePool returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		poolName := "test_pool"
		poolSlots := 5
		poolDescription := "Test pool for unit testing"
		poolIncludeDeferred := true
		mockClient.On("CreatePool", testAirflowURL, airflowclient.Pool{
			Name:            poolName,
			Slots:           poolSlots,
			Description:     poolDescription,
			IncludeDeferred: poolIncludeDeferred,
		}).Return(errTest).Once()

		err := PoolCreate(testAirflowURL, poolName, poolDescription, poolSlots, poolIncludeDeferred, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

func (s *Suite) TestPoolUpdate() {
	s.Run("happy path TestPoolUpdate", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		poolName := "test_pool"
		poolSlots := 5
		poolDescription := "Test pool for unit testing"
		poolIncludeDeferred := true
		mockClient.On("UpdatePool", testAirflowURL, airflowclient.Pool{
			Name:            poolName,
			Slots:           poolSlots,
			Description:     poolDescription,
			IncludeDeferred: poolIncludeDeferred,
		}).Return(nil).Once()

		err := PoolUpdate(testAirflowURL, poolName, poolDescription, poolSlots, poolIncludeDeferred, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when UpdatePool returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)
		poolName := "test_pool"
		poolSlots := 5
		poolDescription := "Test pool for unit testing"
		poolIncludeDeferred := true
		mockClient.On("UpdatePool", testAirflowURL, airflowclient.Pool{
			Name:            poolName,
			Slots:           poolSlots,
			Description:     poolDescription,
			IncludeDeferred: poolIncludeDeferred,
		}).Return(errTest).Once()

		err := PoolUpdate(testAirflowURL, poolName, poolDescription, poolSlots, poolIncludeDeferred, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}

var (
	fromPools = []airflowclient.Pool{
		{Name: "pool1", Slots: 5, Description: "desc1"},
		{Name: "pool2", Slots: 5, Description: "desc2"},
	}
	toPools = []airflowclient.Pool{
		{Name: "pool2", Slots: 5, Description: "desc2"},
		{Name: "pool3", Slots: 5, Description: "desc3"},
	}
)

func (s *Suite) TestCopyPool() {
	s.Run("happy path TestCopyPool", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetPools", fromAirflowURL).Return(airflowclient.Response{Pools: fromPools}, nil).Once()
		mockClient.On("GetPools", toAirflowURL).Return(airflowclient.Response{Pools: toPools}, nil).Once()
		mockClient.On("UpdatePool", toAirflowURL, fromPools[1]).Return(nil).Once()
		mockClient.On("CreatePool", toAirflowURL, fromPools[0]).Return(nil).Once()

		err := CopyPool(fromAirflowURL, toAirflowURL, mockClient, out)
		s.NoError(err)
	})

	s.Run("error path when GetPools returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetPools", fromAirflowURL).Return(airflowclient.Response{}, errTest).Once()

		err := CopyPool(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})

	s.Run("error path when UpdatePool returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetPools", fromAirflowURL).Return(airflowclient.Response{Pools: fromPools}, nil).Once()
		mockClient.On("GetPools", toAirflowURL).Return(airflowclient.Response{Pools: toPools}, nil).Once()
		mockClient.On("CreatePool", toAirflowURL, fromPools[0]).Return(nil).Once()
		mockClient.On("UpdatePool", toAirflowURL, fromPools[1]).Return(errTest).Once()

		err := CopyPool(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})

	s.Run("error path when CreatePool returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(airflowclient_mocks.Client)

		mockClient.On("GetPools", fromAirflowURL).Return(airflowclient.Response{Pools: fromPools}, nil).Once()
		mockClient.On("GetPools", toAirflowURL).Return(airflowclient.Response{Pools: toPools}, nil).Once()
		mockClient.On("CreatePool", toAirflowURL, fromPools[0]).Return(errTest).Once()

		err := CopyPool(fromAirflowURL, toAirflowURL, mockClient, out)
		s.Error(err)
		s.Equal("error", err.Error())
	})
}
