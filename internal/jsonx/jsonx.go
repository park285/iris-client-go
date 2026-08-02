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

// CopyString: 디코드된 string이 입력 버퍼를 alias하지 않게 복사한다. webhook body는
// MaxBytesReader가 감싼 request body이고 decode 후 Close되므로, alias된 string은 해제된
// 버퍼를 가리킬 수 있다.
// ValidateString: 표준 라이브러리처럼 string 값의 unescaped 제어문자(U+0000~U+001F)를
// 거부한다. sonic 기본값은 이 검증을 건너뛴다.
var api = sonic.Config{
	CopyString:     true,
	ValidateString: true,
}.Froze()

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
