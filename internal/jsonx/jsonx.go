package jsonx

import (
	"encoding/json"
	"io"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
)

type (
	SyntaxError        = decoder.SyntaxError
	UnmarshalTypeError = decoder.MismatchTypeError
	RawMessage         = json.RawMessage
	Number             = json.Number
	Encoder            = sonic.Encoder
	Decoder            = sonic.Decoder
)

var api = sonic.ConfigDefault

func Marshal(v any) ([]byte, error) {
	return api.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return api.Unmarshal(data, v)
}

func NewEncoder(w io.Writer) Encoder {
	return api.NewEncoder(w)
}

func NewDecoder(r io.Reader) Decoder {
	return api.NewDecoder(r)
}
