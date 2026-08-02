package jsonx

import (
	"bytes"
	stdjson "encoding/json"
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

func TestDecodeStringValidationMatchesEncodingJSON(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"unescapedControlChar": "{\"name\":\"a\x01b\",\"age\":1}",
		"invalidUTF8":          "{\"name\":\"a\xffb\",\"age\":1}",
		"escapedControlChar":   `{"name":"ab","age":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got samplePayload
			err := Unmarshal([]byte(body), &got)

			var want samplePayload
			wantErr := stdjson.Unmarshal([]byte(body), &want)

			if (err != nil) != (wantErr != nil) {
				t.Fatalf("Unmarshal(%q) error = %v, encoding/json error = %v", body, err, wantErr)
			}
			if err == nil && got != want {
				t.Fatalf("Unmarshal(%q) = %+v, encoding/json = %+v", body, got, want)
			}
		})
	}
}

func TestDecodedStringDoesNotAliasInputBuffer(t *testing.T) {
	t.Parallel()

	buf := []byte(`{"name":"iris","age":1}`)

	var got samplePayload
	if err := Unmarshal(buf, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	clear(buf)

	if got.Name != "iris" {
		t.Fatalf("decoded name = %q after the input buffer was cleared, want %q", got.Name, "iris")
	}
}
