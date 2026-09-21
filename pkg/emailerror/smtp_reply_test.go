package emailerror

import (
	"fmt"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestSMTPReplyClassificationPreservesStage(t *testing.T) {
	for _, tc := range []struct {
		stage   string
		code    int
		message string
		kind    ErrorType
		retry   bool
	}{
		{"RCPT TO", 550, "5.1.1 mailbox unavailable", ErrorTypeRecipient, false},
		{"RCPT TO", 552, "5.2.2 mailbox full", ErrorTypeRecipient, false},
		{"RCPT TO", 450, "4.2.2 mailbox full", ErrorTypeProvider, true},
		{"RCPT TO", 550, "5.7.1 sender blocked", ErrorTypeProvider, false},
		{"RCPT TO", 550, "Rejected by server\n5.7.1 sender blocked", ErrorTypeProvider, false},
		{"RCPT TO", 500, "command not understood", ErrorTypeProvider, false},
		{"MAIL FROM", 550, "sender not authorized", ErrorTypeProvider, false},
		{"DATA", 554, "5.7.1 provider policy", ErrorTypeProvider, false},
		{"DATA", 451, "temporary refusal", ErrorTypeProvider, true},
		{"AUTH", 535, "credentials rejected", ErrorTypeProvider, false},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.stage, tc.code), func(t *testing.T) {
			err := fmt.Errorf("send failed: %w", &SMTPReplyError{Stage: tc.stage, Code: tc.code, Message: tc.message})
			got := NewClassifier().Classify(err, domain.EmailProviderKindSMTP)
			require.Equal(t, tc.kind, got.Type)
			require.Equal(t, tc.retry, got.Retryable)
			require.Contains(t, got.Original.Error(), tc.message)
		})
	}
}
