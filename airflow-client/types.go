package airflowclient

// Connection represents the structure of an Airflow connection
type Connection struct {
	ConnID      string `json:"connection_id"`
	ConnType    string `json:"conn_type"`
	Description string `json:"description"`
	Host        string `json:"host"`
	Schema      string `json:"schema"`
	Login       string `json:"login"`
	Password    string `json:"password"`
	Port        int    `json:"port"`
	Extra       string `json:"extra"`
}

// Variable represents the structure of an Airflow variable
type Variable struct {
	Description string `json:"description"`
	Key         string `json:"key"`
	Value       string `json:"value"`
}

// Pool represents the structure of an Airflow pool
type Pool struct {
	Description     string `json:"description"`
	Name            string `json:"name"`
	Slots           int    `json:"slots"`
	IncludeDeferred bool   `json:"include_deferred"`
}

type Response struct {
	Connections []Connection `json:"connections"`
	Variables   []Variable   `json:"variables"`
	Pools       []Pool       `json:"pools"`
}
