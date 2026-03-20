package client

import "testing"

func TestNormalizeReplyThreadID(t *testing.T) {
	valid := "12345"
	spaced := "  67890\t"
	empty := ""
	spaces := " \n\t "
	invalid := "12a45"

	tests := []struct {
		name  string
		input *string
		want  *string
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "valid numeric thread id",
			input: &valid,
			want:  &valid,
		},
		{
			name:  "trim whitespace",
			input: &spaced,
			want:  stringPtr("67890"),
		},
		{
			name:  "empty string becomes nil",
			input: &empty,
			want:  nil,
		},
		{
			name:  "whitespace only becomes nil",
			input: &spaces,
			want:  nil,
		},
		{
			name:  "non numeric becomes nil",
			input: &invalid,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReplyThreadID(tt.input)
			if !equalStringPtr(got, tt.want) {
				t.Fatalf("normalizeReplyThreadID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeReplyThreadScope(t *testing.T) {
	zero := 0
	negative := -1
	positive := 2

	tests := []struct {
		name  string
		input *int
		want  *int
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "zero becomes nil",
			input: &zero,
			want:  nil,
		},
		{
			name:  "negative becomes nil",
			input: &negative,
			want:  nil,
		},
		{
			name:  "positive remains set",
			input: &positive,
			want:  &positive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReplyThreadScope(tt.input)
			if !equalIntPtr(got, tt.want) {
				t.Fatalf("normalizeReplyThreadScope() = %v, want %v", got, tt.want)
			}
		})
	}
}
