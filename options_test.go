package iris

import "testing"

func TestApplySendOptions(t *testing.T) {
	threadID := "12345"
	threadScope := 2

	tests := []struct {
		name string
		opts []SendOption
		want sendOptions
	}{
		{
			name: "empty options",
			opts: nil,
			want: sendOptions{},
		},
		{
			name: "thread id only",
			opts: []SendOption{WithThreadID(threadID)},
			want: sendOptions{ThreadID: &threadID},
		},
		{
			name: "thread scope only",
			opts: []SendOption{WithThreadScope(threadScope)},
			want: sendOptions{ThreadScope: &threadScope},
		},
		{
			name: "both options",
			opts: []SendOption{WithThreadID(threadID), WithThreadScope(threadScope)},
			want: sendOptions{ThreadID: &threadID, ThreadScope: &threadScope},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSendOptionsEqual(t, ApplySendOptions(tt.opts), tt.want)
		})
	}
}

func TestValidateSendOptionsValidCases(t *testing.T) {
	threadID := "12345"
	one := 1
	two := 2

	tests := []validateSendOptionsSuccessCase{
		{
			name:  "empty options",
			input: sendOptions{},
		},
		{
			name:  "valid thread id only",
			input: sendOptions{ThreadID: &threadID},
		},
		{
			name:  "valid scope one without thread id",
			input: sendOptions{ThreadScope: &one},
		},
		{
			name:  "valid scope two with thread id",
			input: sendOptions{ThreadID: &threadID, ThreadScope: &two},
		},
	}

	runValidateSendOptionsSuccessTests(t, tests)
}

func TestValidateSendOptionsInvalidCases(t *testing.T) {
	threadID := "12a45"
	zero := 0
	negative := -1
	two := 2

	tests := []validateSendOptionsErrorCase{
		{
			name:    "reject non numeric thread id",
			input:   sendOptions{ThreadID: &threadID},
			wantErr: `iris: threadId must be numeric, got "12a45"`,
		},
		{
			name:    "reject zero thread scope",
			input:   sendOptions{ThreadScope: &zero},
			wantErr: "iris: threadScope must be positive, got 0",
		},
		{
			name:    "reject negative thread scope",
			input:   sendOptions{ThreadScope: &negative},
			wantErr: "iris: threadScope must be positive, got -1",
		},
		{
			name:    "reject scope two without thread id",
			input:   sendOptions{ThreadScope: &two},
			wantErr: "iris: threadScope >= 2 requires threadId",
		},
	}

	runValidateSendOptionsErrorTests(t, tests)
}

type validateSendOptionsSuccessCase struct {
	name  string
	input sendOptions
}

func runValidateSendOptionsSuccessTests(t *testing.T, tests []validateSendOptionsSuccessCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSendOptions(tt.input); err != nil {
				t.Fatalf("ValidateSendOptions() error = %v, want nil", err)
			}
		})
	}
}

type validateSendOptionsErrorCase struct {
	name    string
	input   sendOptions
	wantErr string
}

func runValidateSendOptionsErrorTests(t *testing.T, tests []validateSendOptionsErrorCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSendOptions(tt.input)
			if err == nil {
				t.Fatalf("ValidateSendOptions() error = nil, want %q", tt.wantErr)
			}

			if err.Error() != tt.wantErr {
				t.Fatalf("ValidateSendOptions() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func assertSendOptionsEqual(t *testing.T, got, want sendOptions) {
	t.Helper()

	if !equalStringPtr(got.ThreadID, want.ThreadID) {
		t.Fatalf("ThreadID = %v, want %v", got.ThreadID, want.ThreadID)
	}

	if !equalIntPtr(got.ThreadScope, want.ThreadScope) {
		t.Fatalf("ThreadScope = %v, want %v", got.ThreadScope, want.ThreadScope)
	}
}

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
			got := NormalizeReplyThreadID(tt.input)
			if !equalStringPtr(got, tt.want) {
				t.Fatalf("NormalizeReplyThreadID() = %v, want %v", got, tt.want)
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
			got := NormalizeReplyThreadScope(tt.input)
			if !equalIntPtr(got, tt.want) {
				t.Fatalf("NormalizeReplyThreadScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func equalStringPtr(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func equalIntPtr(got, want *int) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}
