package service

import (
	"net/url"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mailgunInboundForm(messageHeaders string) url.Values {
	f := url.Values{}
	f.Set("sender", "jane@example.com")
	f.Set("from", "Jane Doe <Jane@Example.com>")
	f.Set("recipient", "hello@mg.example.com")
	f.Set("subject", "Re: Welcome aboard")
	f.Set("timestamp", "1700000000")
	f.Set("message-headers", messageHeaders)
	return f
}

func TestMailgunReplyParser_Parse_Genuine(t *testing.T) {
	p := &MailgunReplyParser{}
	headers := `[["From","Jane Doe <Jane@Example.com>"],["Message-Id","<reply-1@mail.example.com>"],["In-Reply-To","<orig-123@example.com>"],["References","<a@x.com> <orig-123@example.com>"]]`
	reply, err := p.Parse(&domain.InboundRequest{Form: mailgunInboundForm(headers)})
	require.NoError(t, err)

	assert.Equal(t, "jane@example.com", reply.FromEmail)
	assert.Equal(t, "hello@mg.example.com", reply.ToEmail)
	assert.Equal(t, "Re: Welcome aboard", reply.Subject)
	assert.Equal(t, "reply-1@mail.example.com", reply.MessageID)
	assert.Equal(t, "orig-123@example.com", reply.InReplyTo)
	assert.Equal(t, []string{"a@x.com", "orig-123@example.com"}, reply.References)
	assert.Equal(t, int64(1700000000), reply.ReceivedAt.Unix())
	assert.Equal(t, domain.ReplyGenuine, domain.Classify(reply))
}

func TestMailgunReplyParser_Parse_FromHeaderFallbackAndLowercase(t *testing.T) {
	p := &MailgunReplyParser{}
	f := url.Values{}
	// No "sender" → fall back to the From header, and lowercase.
	f.Set("from", "Jane Doe <Jane@Example.COM>")
	f.Set("message-headers", `[["Message-Id","<r2@x>"]]`)
	reply, err := p.Parse(&domain.InboundRequest{Form: f})
	require.NoError(t, err)
	assert.Equal(t, "jane@example.com", reply.FromEmail)
	assert.Equal(t, "r2@x", reply.MessageID)
}

func TestMailgunReplyParser_Parse_MissingSender(t *testing.T) {
	p := &MailgunReplyParser{}
	_, err := p.Parse(&domain.InboundRequest{Form: url.Values{}})
	assert.Error(t, err)
}

func TestMailgunReplyParser_Parse_ClassifiesAutoReplyAndBounce(t *testing.T) {
	p := &MailgunReplyParser{}

	ooo, err := p.Parse(&domain.InboundRequest{Form: mailgunInboundForm(`[["Auto-Submitted","auto-replied"],["Message-Id","<o1@x>"]]`)})
	require.NoError(t, err)
	assert.Equal(t, domain.ReplyAutoResponder, domain.Classify(ooo))

	bounce, err := p.Parse(&domain.InboundRequest{Form: mailgunInboundForm(`[["Content-Type","multipart/report; report-type=delivery-status"]]`)})
	require.NoError(t, err)
	assert.Equal(t, domain.ReplyBounce, domain.Classify(bounce))
}

func TestMailgunReplyParser_Verify_NoOp(t *testing.T) {
	p := &MailgunReplyParser{}
	integ := &domain.Integration{EmailProvider: domain.EmailProvider{Mailgun: &domain.MailgunSettings{APIKey: "key-abc"}}}

	// Signature verification is intentionally disabled: Verify always passes, even with
	// no/garbage signature fields. Reply authenticity rests on Message-ID matching.
	assert.NoError(t, p.Verify(&domain.InboundRequest{Form: url.Values{}}, integ))
	assert.NoError(t, p.Verify(&domain.InboundRequest{Form: url.Values{"signature": {"garbage"}}}, integ))
}
