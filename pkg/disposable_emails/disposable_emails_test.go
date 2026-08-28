package disposable_emails

import (
	"testing"
)

func TestIsDisposableEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{email: "test@example.com", want: false},
		{email: "0-180.com", want: true},
	}

	for _, test := range tests {
		if got := IsDisposableEmail(test.email); got != test.want {
			t.Errorf("IsDisposableEmail(%q) = %v, want %v", test.email, got, test.want)
		}
	}
}
