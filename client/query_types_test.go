package client

import (
	"encoding/json"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestQueryRequestJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    QueryRequest
		wantJSON string
	}{
		{
			name:     "query only, omit bind and decrypt",
			input:    QueryRequest{Query: "SELECT 1"},
			wantJSON: `{"query":"SELECT 1"}`,
		},
		{
			name: "with bind and decrypt",
			input: QueryRequest{
				Query:   "SELECT * FROM t WHERE id = ?",
				Bind:    []json.RawMessage{json.RawMessage(`42`)},
				Decrypt: true,
			},
			wantJSON: `{"query":"SELECT * FROM t WHERE id = ?","bind":[42],"decrypt":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonx.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Fatalf("Marshal() = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}

func TestQueryResponseJSON(t *testing.T) {
	raw := `{
		"rowCount": 2,
		"columns": [
			{"name": "id", "sqliteType": "INTEGER"},
			{"name": "name", "sqliteType": "TEXT"}
		],
		"rows": [
			[1, "alice"],
			[2, "bob"]
		]
	}`

	var got QueryResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", got.RowCount)
	}
	if len(got.Columns) != 2 {
		t.Fatalf("len(Columns) = %d, want 2", len(got.Columns))
	}
	if got.Columns[0].Name != "id" || got.Columns[0].SQLiteType != "INTEGER" {
		t.Fatalf("Columns[0] = %+v, unexpected", got.Columns[0])
	}
	if got.Columns[1].Name != "name" || got.Columns[1].SQLiteType != "TEXT" {
		t.Fatalf("Columns[1] = %+v, unexpected", got.Columns[1])
	}
	if len(got.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(got.Rows))
	}
	if len(got.Rows[0]) != 2 {
		t.Fatalf("len(Rows[0]) = %d, want 2", len(got.Rows[0]))
	}

	// Verify raw values
	if string(got.Rows[0][0]) != "1" {
		t.Fatalf("Rows[0][0] = %s, want 1", got.Rows[0][0])
	}
	if string(got.Rows[0][1]) != `"alice"` {
		t.Fatalf("Rows[0][1] = %s, want \"alice\"", got.Rows[0][1])
	}
}

func TestQueryRequestRoundTrip(t *testing.T) {
	input := QueryRequest{
		Query:   "SELECT * FROM t WHERE id = ?",
		Bind:    []json.RawMessage{json.RawMessage(`42`)},
		Decrypt: true,
	}

	data, err := jsonx.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got QueryRequest
	if err := jsonx.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Query != input.Query {
		t.Fatalf("Query = %q, want %q", got.Query, input.Query)
	}
	if got.Decrypt != input.Decrypt {
		t.Fatalf("Decrypt = %v, want %v", got.Decrypt, input.Decrypt)
	}
	if len(got.Bind) != 1 || string(got.Bind[0]) != "42" {
		t.Fatalf("Bind = %v, want [42]", got.Bind)
	}
}
