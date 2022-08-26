package settings

// Connections is an array of airflow connection
type Connections []Connection

// Connection contains structure of airflow connection
type Connection struct {
	Conn_ID       string      `mapstructure:"conn_id"`
	Conn_Type     string      `mapstructure:"conn_type"`
	Conn_Host     string      `mapstructure:"conn_host"`
	Conn_Schema   string      `mapstructure:"conn_schema"`
	Conn_Login    string      `mapstructure:"conn_login"`
	Conn_Password string      `mapstructure:"conn_password"`
	Conn_Port     int         `mapstructure:"conn_port"`
	Conn_URI      string      `mapstructure:"conn_uri"`
	Conn_Extra    interface{} `mapstructure:"conn_extra"`
}

// Pools contains structure of airflow pools
type Pools []struct {
	Pool_Name        string `mapstructure:"pool_name"`
	Pool_Slot        int    `mapstructure:"pool_slot"`
	Pool_Description string `mapstructure:"pool_description"`
}

// Variables contains structure of airflow variables
type Variables []struct {
	Variable_Name  string `mapstructure:"variable_name"`
	Variable_Value string `mapstructure:"variable_value"`
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

type ListConnections []ListConnection

type ListConnection struct {
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

type ListPools []struct {
	PoolName        string `yaml:"pool"`
	PoolSlot        string `yaml:"slots"`
	PoolDescription string `yaml:"description"`
}
