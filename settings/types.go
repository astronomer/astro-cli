package settings

type Connections []Connection

// Connection contains structure of airflow connection
type Connection struct {
	ConnID       string `mapstructure:"conn_id"`
	ConnType     string `mapstructure:"conn_type"`
	ConnHost     string `mapstructure:"conn_host"`
	ConnSchema   string `mapstructure:"conn_schema"`
	ConnLogin    string `mapstructure:"conn_login"`
	ConnPassword string `mapstructure:"conn_password"`
	ConnPort     int    `mapstructure:"conn_port"`
	ConnURI      string `mapstructure:"conn_uri"`
	ConnExtra    string `mapstructure:"conn_extra"`
}

// Pools contains structure of airflow pools
type Pools []struct {
	PoolName        string `mapstructure:"pool_name"`
	PoolSlot        int    `mapstructure:"pool_slot"`
	PoolDescription string `mapstructure:"pool_description"`
}

// Variables contains structure of airflow variables
type Variables []struct {
	VariableName  string `mapstructure:"variable_name"`
	VariableValue string `mapstructure:"variable_value"`
}

// Airflow contains structure of airflow settings
type Airflow struct {
	Connections `mapstructure:"connections"`
	Pools       `mapstructure:"pools"`
	Variables   `mapstructure:"variables"`
}

// Config is input data to generate connections, pools, and variables
type Config struct {
	Airflow `mapstructure:"airflow"`
}
