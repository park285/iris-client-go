package transport

import "testing"

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
		{"TransportH3", TransportH3, "h3"},
		{"TransportHTTP1", TransportHTTP1, "http1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("wire constant %s drift: got %q want %q", c.name, c.got, c.want)
		}
	}
}
