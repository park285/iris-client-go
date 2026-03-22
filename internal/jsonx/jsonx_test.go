package jsonx

import (
	"bytes"
	"testing"
)

type samplePayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestMarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	input := samplePayload{Name: "iris", Age: 7}

	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got samplePayload
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got != input {
		t.Fatalf("roundtrip = %+v, want %+v", got, input)
	}
}

func TestEncoderAndDecoder(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	input := samplePayload{Name: "sonic", Age: 1}
	if err := enc.Encode(input); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	dec := NewDecoder(&buf)

	var got samplePayload
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got != input {
		t.Fatalf("decoded = %+v, want %+v", got, input)
	}
}
