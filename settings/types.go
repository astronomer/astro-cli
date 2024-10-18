package settings

// Connections is an array of airflow connection
type Connections []Connection

// Connection contains structure of airflow connection
type Connection struct {
	ConnID       string      `mapstructure:"conn_id" yaml:"conn_id"`
	ConnType     string      `mapstructure:"conn_type" yaml:"conn_type"`
	ConnHost     string      `mapstructure:"conn_host" yaml:"conn_host"`
	ConnSchema   string      `mapstructure:"conn_schema" yaml:"conn_schema"`
	ConnLogin    string      `mapstructure:"conn_login" yaml:"conn_login"`
	ConnPassword string      `mapstructure:"conn_password" yaml:"conn_password"`
	ConnPort     int         `mapstructure:"conn_port" yaml:"conn_port"`
	ConnURI      string      `mapstructure:"conn_uri" yaml:"conn_uri"`
	ConnExtra    interface{} `mapstructure:"conn_extra" yaml:"conn_extra"`
}

// Pools contains structure of airflow pools
type Pools []struct {
	PoolName            string `mapstructure:"pool_name" yaml:"pool_name"`
	PoolSlot            int    `mapstructure:"pool_slot" yaml:"pool_slot"`
	PoolDescription     string `mapstructure:"pool_description" yaml:"pool_description"`
	PoolIncludeDeferred bool   `mapstructure:"pool_include_deferred" yaml:"pool_include_deferred"`
}

// Variables contains structure of airflow variables
type Variables []struct {
	VariableName  string `mapstructure:"variable_name" yaml:"variable_name"`
	VariableValue string `mapstructure:"variable_value" yaml:"variable_value"`
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

type AirflowConnections []AirflowConnection

type AirflowConnection struct {
	ConnID       string            `yaml:"conn_id"`
	ConnType     string            `yaml:"conn_type"`
	ConnHost     string            `yaml:"host"`
	ConnSchema   string            `yaml:"schema"`
	ConnLogin    string            `yaml:"login"`
	ConnPassword string            `yaml:"password"`
	ConnPort     string            `yaml:"port"`
	ConnURI      string            `yaml:"get_uri"`
	ConnExtra    map[string]string `yaml:"extra_dejson"`
}

type AirflowPools []struct {
	PoolName            string `yaml:"pool"`
	PoolSlot            string `yaml:"slots"`
	PoolDescription     string `yaml:"description"`
	PoolIncludeDeferred bool   `yaml:"include_deferred"`
}

// types for creating variables and connections yaml files

type DAGRunVariables struct {
	VarYAMLs `mapstructure:"connections" yaml:"variables"`
}

type VarYAMLs []VarYAML

type VarYAML struct {
	Key   string `mapstructure:"key" yaml:"key"`
	Value string `mapstructure:"value" yaml:"value"`
}

type DAGRunConnections struct {
	ConnYAMLs `mapstructure:"connections" yaml:"connections"`
}

type ConnYAMLs []ConnYAML

type ConnYAML struct {
	ConnID   string      `yaml:"conn_id"`
	ConnType string      `yaml:"conn_type"`
	Host     string      `yaml:"host"`
	Schema   string      `yaml:"schema"`
	Login    string      `yaml:"login"`
	Password string      `yaml:"password"`
	Port     int         `yaml:"port"`
	Extra    interface{} `yaml:"extra"`
}
