package client

import "encoding/json"

type QueryRequest struct {
	Query   string            `json:"query"`
	Bind    []json.RawMessage `json:"bind,omitempty"`
	Decrypt bool              `json:"decrypt,omitempty"`
}

type QueryColumn struct {
	Name       string `json:"name"`
	SQLiteType string `json:"sqliteType"`
}

type QueryResponse struct {
	RowCount int                 `json:"rowCount"`
	Columns  []QueryColumn       `json:"columns"`
	Rows     [][]json.RawMessage `json:"rows"`
}
