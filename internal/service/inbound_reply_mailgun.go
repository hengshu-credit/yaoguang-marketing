package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// MailgunReplyParser parses and authenticates inbound reply messages forwarded by a
// Mailgun Route's forward() action (application/x-www-form-urlencoded or
// multipart/form-data). It implements domain.ReplyParser.
type MailgunReplyParser struct{}

// Source identifies the provider this parser handles.
func (p *MailgunReplyParser) Source() domain.WebhookSource { return domain.WebhookSourceMailgun }

// Verify is intentionally a no-op for Mailgun: the inbound webhook payload signature
// is not checked. A reply is matched to a send by the recipient-visible RFC Message-ID
// (In-Reply-To/References → stored smtp_message_id), which is unguessable — forging an
// exit would require knowing a real send's Message-ID, so the Message-ID itself is the
// shared secret. The signing key (Mailgun's separate HTTP webhook signing key) is not
// stored, so HMAC verification is deliberately omitted.
func (p *MailgunReplyParser) Verify(req *domain.InboundRequest, integration *domain.Integration) error {
	return nil
}

// Parse builds a canonical InboundReply from a Mailgun forward() POST. Base fields
// are form values; threading/classification headers (Message-Id, In-Reply-To,
// References, Auto-Submitted, Precedence, Content-Type) live in "message-headers".
func (p *MailgunReplyParser) Parse(req *domain.InboundRequest) (*domain.InboundReply, error) {
	if req == nil || req.Form == nil {
		return nil, fmt.Errorf("mailgun inbound: empty form")
	}
	form := req.Form
	headers := parseMailgunHeaders(form.Get("message-headers"))

	// "sender" is the SMTP envelope sender (bare addr); fall back to the From header.
	sender := strings.TrimSpace(form.Get("sender"))
	if sender == "" {
		sender = form.Get("from")
	}
	fromEmail := parseReplyAddress(sender)
	if fromEmail == "" {
		fromEmail = parseReplyAddress(headers["from"])
	}
	if fromEmail == "" {
		return nil, fmt.Errorf("mailgun inbound: missing sender address")
	}

	receivedAt := time.Now().UTC()
	if ts := form.Get("timestamp"); ts != "" {
		if secs, err := strconv.ParseInt(ts, 10, 64); err == nil {
			receivedAt = time.Unix(secs, 0).UTC()
		}
	}

	return &domain.InboundReply{
		FromEmail:     strings.ToLower(fromEmail),
		ToEmail:       strings.ToLower(parseReplyAddress(form.Get("recipient"))),
		Subject:       form.Get("subject"),
		MessageID:     stripAngle(firstNonEmpty(form.Get("Message-Id"), headers["message-id"])),
		InReplyTo:     firstMessageID(headers["in-reply-to"]),
		References:    parseMessageIDList(headers["references"]),
		ReceivedAt:    receivedAt,
		AutoSubmitted: headers["auto-submitted"],
		Precedence:    headers["precedence"],
		ContentType:   headers["content-type"],
		RawHeaders:    headers,
	}, nil
}
