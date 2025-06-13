package deployment

import (
	"fmt"
	"io"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/util"
)

// Variable epresents the structure of an Airflow variable
type Pool struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	Slots       int    `json:"slots"`
}

type PoolResp struct {
	Pools []Pool `json:"pools"`
}

func ConnectionList(airflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	conTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"CONNECTION ID", "CONN TYPE"},
	}

	// get connectons
	resp, err := airflowAPIClient.GetConnections(airflowURL)
	if err != nil {
		return err
	}
	// Print Connections
	for i := range resp.Connections {
		conn := resp.Connections[i]
		conTab.AddRow([]string{conn.ConnID, conn.ConnType}, false)
	}

	return conTab.Print(out)
}

func ConnectionCreate(airflowURL, connID, connType, description, host, login, password, schema, extra string, port int, airflowAPIClient airflowclient.Client, out io.Writer) error {
	connection := airflowclient.Connection{
		ConnID:      connID,
		ConnType:    connType,
		Description: description,
		Host:        host,
		Schema:      schema,
		Login:       login,
		Password:    password,
		Port:        port,
		Extra:       extra,
	}

	// create connections
	fmt.Printf("Creating connection %s\n", connID)
	err := airflowAPIClient.CreateConnection(airflowURL, &connection)
	if err != nil {
		return err
	}
	return nil
}

func ConnectionUpdate(airflowURL, connID, connType, description, host, login, password, schema, extra string, port int, airflowAPIClient airflowclient.Client, out io.Writer) error {
	connection := airflowclient.Connection{
		ConnID:      connID,
		ConnType:    connType,
		Description: description,
		Host:        host,
		Schema:      schema,
		Login:       login,
		Password:    password,
		Port:        port,
		Extra:       extra,
	}

	// update connection
	fmt.Printf("updating connection %s\n", connID)
	err := airflowAPIClient.UpdateConnection(airflowURL, &connection)
	if err != nil {
		return err
	}
	return nil
}

//nolint:dupl
func CopyConnection(fromAirflowURL, toAirflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	// get connectons from original Deployment
	fromConnectionResp, err := airflowAPIClient.GetConnections(fromAirflowURL)
	if err != nil {
		return err
	}

	toConnectionResp, err := airflowAPIClient.GetConnections(toAirflowURL)
	if err != nil {
		return err
	}
	toConnIds := make([]string, 0, len(toConnectionResp.Connections))
	for i := range toConnectionResp.Connections {
		toConnIds = append(toConnIds, toConnectionResp.Connections[i].ConnID)
	}

	// push connection to new deployment
	for i := range fromConnectionResp.Connections {
		fmt.Printf("Copying Connecton %s\n", fromConnectionResp.Connections[i].ConnID)
		// create or update connection
		if util.Contains(toConnIds, fromConnectionResp.Connections[i].ConnID) {
			err = airflowAPIClient.UpdateConnection(toAirflowURL, &fromConnectionResp.Connections[i])
			if err != nil {
				return err
			}
		} else {
			err = airflowAPIClient.CreateConnection(toAirflowURL, &fromConnectionResp.Connections[i])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AirflowVariableList(airflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	conTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"KEY", "DESCRIPTION"},
	}

	// get variables
	variablesResp, err := airflowAPIClient.GetVariables(airflowURL)
	if err != nil {
		return err
	}
	// Print Connections
	for i := range variablesResp.Variables {
		variable := variablesResp.Variables[i]
		conTab.AddRow([]string{variable.Key, variable.Description}, false)
	}

	return conTab.Print(out)
}

func VariableCreate(airflowURL, value, key, description string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	variable := airflowclient.Variable{
		Key:         key,
		Value:       value,
		Description: description,
	}

	// create connections
	fmt.Printf("Creating variable %s\n", variable.Key)
	err := airflowAPIClient.CreateVariable(airflowURL, variable)
	if err != nil {
		return err
	}
	return nil
}

func VariableUpdate(airflowURL, value, key, description string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	variable := airflowclient.Variable{
		Key:         key,
		Value:       value,
		Description: description,
	}

	// update connection
	fmt.Printf("updating variable %s\n", variable.Key)
	err := airflowAPIClient.UpdateVariable(airflowURL, variable)
	if err != nil {
		return err
	}
	return nil
}

//nolint:dupl
func CopyVariable(fromAirflowURL, toAirflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	// get variables from original Deployment
	fromVariableResp, err := airflowAPIClient.GetVariables(fromAirflowURL)
	if err != nil {
		return err
	}

	toVariableResp, err := airflowAPIClient.GetVariables(toAirflowURL)
	if err != nil {
		return err
	}
	toVarKeys := make([]string, 0, len(toVariableResp.Variables))
	for i := range toVariableResp.Variables {
		toVarKeys = append(toVarKeys, toVariableResp.Variables[i].Key)
	}

	// push variable to new deployment
	for i := range fromVariableResp.Variables {
		fmt.Printf("Copying Variable %s\n", fromVariableResp.Variables[i].Key)
		// create or update variable
		if util.Contains(toVarKeys, fromVariableResp.Variables[i].Key) {
			err = airflowAPIClient.UpdateVariable(toAirflowURL, fromVariableResp.Variables[i])
			if err != nil {
				return err
			}
		} else {
			err = airflowAPIClient.CreateVariable(toAirflowURL, fromVariableResp.Variables[i])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func PoolList(airflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	conTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "SLOTS", "INCLUDE DEFERRED"},
	}

	// get pools
	poolsResp, err := airflowAPIClient.GetPools(airflowURL)
	if err != nil {
		return err
	}
	// Print pools
	for i := range poolsResp.Pools {
		pool := poolsResp.Pools[i]
		conTab.AddRow([]string{pool.Name, fmt.Sprint(pool.Slots), fmt.Sprint(pool.IncludeDeferred)}, false)
	}

	return conTab.Print(out)
}

func PoolCreate(airflowURL, name, description string, slots int, includeDeferred bool, airflowAPIClient airflowclient.Client, out io.Writer) error {
	pool := airflowclient.Pool{
		Name:            name,
		Slots:           slots,
		Description:     description,
		IncludeDeferred: includeDeferred,
	}

	fmt.Printf("Creating pool %s\n", pool.Name)
	err := airflowAPIClient.CreatePool(airflowURL, pool)
	if err != nil {
		return err
	}
	return nil
}

func PoolUpdate(airflowURL, name, description string, slots int, includeDeferred bool, airflowAPIClient airflowclient.Client, out io.Writer) error {
	pool := airflowclient.Pool{
		Name:            name,
		Slots:           slots,
		Description:     description,
		IncludeDeferred: includeDeferred,
	}

	fmt.Printf("updating pool %s\n", pool.Name)
	err := airflowAPIClient.UpdatePool(airflowURL, pool)
	if err != nil {
		return err
	}
	return nil
}

//nolint:dupl
func CopyPool(fromAirflowURL, toAirflowURL string, airflowAPIClient airflowclient.Client, out io.Writer) error {
	// get Pools from original Deployment
	fromPoolResp, err := airflowAPIClient.GetPools(fromAirflowURL)
	if err != nil {
		return err
	}

	toPoolResp, err := airflowAPIClient.GetPools(toAirflowURL)
	if err != nil {
		return err
	}
	toPoolKeys := make([]string, 0, len(toPoolResp.Pools))
	for i := range toPoolResp.Pools {
		toPoolKeys = append(toPoolKeys, toPoolResp.Pools[i].Name)
	}

	// push pool to new deployment
	for i := range fromPoolResp.Pools {
		fmt.Printf("Copying Pool %s\n", fromPoolResp.Pools[i].Name)
		// create or update pool
		if util.Contains(toPoolKeys, fromPoolResp.Pools[i].Name) {
			err = airflowAPIClient.UpdatePool(toAirflowURL, fromPoolResp.Pools[i])
			if err != nil {
				return err
			}
		} else {
			err = airflowAPIClient.CreatePool(toAirflowURL, fromPoolResp.Pools[i])
			if err != nil {
				return err
			}
		}
	}
	return nil
}
