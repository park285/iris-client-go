package transport

import "testing"

// 각 상수를 원본 wire 리터럴에 고정한다. 값이 바뀌면 서비스 간 메시지
// 라우팅·transport 협상이 조용히 깨지므로 빌드를 실패시킨다. 상수당 독립
// 항목으로 검사해 두 상수가 같은 값으로 수렴해도 체크가 소실되지 않게 한다.
func TestWireConstantValuesAreStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"msgTypeText", msgTypeText, "text"},
		{"msgTypeImage", msgTypeImage, "image"},
		{"msgTypeImageMultiple", msgTypeImageMultiple, "image_multiple"},
		{"mimeImagePNG", mimeImagePNG, "image/png"},
		{"transportH3", transportH3, "h3"},
		{"transportH2C", transportH2C, "h2c"},
		{"transportHTTP2", transportHTTP2, "http2"},
		{"transportHTTP1", transportHTTP1, "http1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("wire constant %s drift: got %q want %q", c.name, c.got, c.want)
		}
	}
}
