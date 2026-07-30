package airflowapi

import "context"

// Connection is an Airflow connection. The password is deliberately not a
// field: the API sends it and this client never decodes it, so it cannot
// reach a log or a table by accident. A caller that genuinely needs it goes
// through Do.
type Connection struct {
	ConnectionID string `json:"connection_id"`
	ConnType     string `json:"conn_type"`
	Description  string `json:"description"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Schema       string `json:"schema"`
	Login        string `json:"login"`
	Extra        string `json:"extra"`
}

// ConnectionList is a page of connections.
type ConnectionList struct {
	Connections  []Connection `json:"connections"`
	TotalEntries int          `json:"total_entries"`
}

// ListConnections lists connections.
func (c *Client) ListConnections(ctx context.Context, opts ListOptions) (ConnectionList, error) {
	var list ConnectionList
	err := c.getCollection(ctx, "/connections", opts.query(), &list)
	return list, err
}

// GetConnection reads one connection.
func (c *Client) GetConnection(ctx context.Context, connectionID string) (Connection, error) {
	var connection Connection
	err := c.get(ctx, pathf("/connections/%s", connectionID), nil, &connection)
	return connection, err
}

// Variable is an Airflow variable.
type Variable struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	// IsEncrypted is Airflow 3 only.
	IsEncrypted bool `json:"is_encrypted"`
}

// VariableList is a page of variables.
type VariableList struct {
	Variables    []Variable `json:"variables"`
	TotalEntries int        `json:"total_entries"`
}

// ListVariables lists variables. Airflow masks the values of variables whose
// names look sensitive; nothing here unmasks them.
func (c *Client) ListVariables(ctx context.Context, opts ListOptions) (VariableList, error) {
	var list VariableList
	err := c.getCollection(ctx, "/variables", opts.query(), &list)
	return list, err
}

// GetVariable reads one variable.
func (c *Client) GetVariable(ctx context.Context, key string) (Variable, error) {
	var variable Variable
	err := c.get(ctx, pathf("/variables/%s", key), nil, &variable)
	return variable, err
}

// Pool is a concurrency pool.
type Pool struct {
	Name            string `json:"name"`
	Slots           int    `json:"slots"`
	OccupiedSlots   int    `json:"occupied_slots"`
	RunningSlots    int    `json:"running_slots"`
	QueuedSlots     int    `json:"queued_slots"`
	ScheduledSlots  int    `json:"scheduled_slots"`
	OpenSlots       int    `json:"open_slots"`
	DeferredSlots   int    `json:"deferred_slots"`
	Description     string `json:"description"`
	IncludeDeferred bool   `json:"include_deferred"`
}

// PoolList is a page of pools.
type PoolList struct {
	Pools        []Pool `json:"pools"`
	TotalEntries int    `json:"total_entries"`
}

// ListPools lists pools.
func (c *Client) ListPools(ctx context.Context, opts ListOptions) (PoolList, error) {
	var list PoolList
	err := c.getCollection(ctx, "/pools", opts.query(), &list)
	return list, err
}

// GetPool reads one pool.
func (c *Client) GetPool(ctx context.Context, name string) (Pool, error) {
	var pool Pool
	err := c.get(ctx, pathf("/pools/%s", name), nil, &pool)
	return pool, err
}
