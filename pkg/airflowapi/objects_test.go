package airflowapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestListConnectionsNeverDecodesThePassword(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/connections",
		`{"connections":[{"connection_id":"warehouse","conn_type":"postgres","host":"db","port":5432,"login":"admin","password":"hunter2"}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ListConnections(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Connections) != 1 {
		t.Fatalf("list = %+v, want the one connection", list)
	}
	connection := list.Connections[0]
	if connection.ConnectionID != "warehouse" || connection.Port != 5432 || connection.Login != "admin" {
		t.Errorf("connection = %+v, want the metadata", connection)
	}
	if strings.Contains(fmt.Sprintf("%+v", connection), "hunter2") {
		t.Error("the password reached the typed connection")
	}
}

func TestGetConnectionReadsOne(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/connections/warehouse", `{"connection_id":"warehouse","conn_type":"postgres"}`)
	client := stub.client()

	connection, err := client.GetConnection(t.Context(), "warehouse")
	if err != nil {
		t.Fatal(err)
	}
	if connection.ConnType != "postgres" {
		t.Errorf("connection = %+v, want the type", connection)
	}
}

func TestVariablesListAndGet(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/variables",
		`{"variables":[{"key":"region","value":"us","is_encrypted":true}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v2/variables/region", `{"key":"region","value":"us"}`)
	client := stub.client()

	list, err := client.ListVariables(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Variables) != 1 || !list.Variables[0].IsEncrypted {
		t.Fatalf("list = %+v, want the variable", list)
	}
	variable, err := client.GetVariable(t.Context(), "region")
	if err != nil {
		t.Fatal(err)
	}
	if variable.Value != "us" {
		t.Errorf("variable = %+v, want its value", variable)
	}
}

func TestPoolsListAndGet(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/pools",
		`{"pools":[{"name":"default_pool","slots":128,"open_slots":127}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v1/pools/default_pool", `{"name":"default_pool","slots":128}`)
	client := stub.client()

	list, err := client.ListPools(t.Context(), ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Pools) != 1 || list.Pools[0].OpenSlots != 127 {
		t.Fatalf("list = %+v, want the pool", list)
	}
	pool, err := client.GetPool(t.Context(), "default_pool")
	if err != nil {
		t.Fatal(err)
	}
	if pool.Slots != 128 {
		t.Errorf("pool = %+v, want its slots", pool)
	}
}

func TestGetVariableReportsAMissingKey(t *testing.T) {
	stub := newAF3Stub(t)
	client := stub.client()

	_, err := client.GetVariable(t.Context(), "absent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want it to read as not found", err)
	}
	if errors.Is(err, ErrNotServed) {
		t.Error("a missing key is not a missing endpoint")
	}
}
