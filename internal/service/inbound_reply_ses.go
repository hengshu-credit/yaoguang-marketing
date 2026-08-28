package service

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SNS SignatureVersion 1 mandates SHA-1; v2 (SHA-256) also supported.
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// snsHostRe matches a legitimate Amazon SNS host for SigningCertURL / SubscribeURL, so a
// forged message can't point us at an arbitrary host (defence in depth alongside the
// SSRF-safe HTTP client and the signature check).
var snsHostRe = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

const sesInboundFetchTimeout = 10 * time.Second

// SESReplyParser parses and authenticates inbound reply messages that Amazon SES delivers
// via an SNS topic (a receipt rule's SNS action). It implements domain.ReplyParser.
//
// SES wraps each received email in an SNS envelope (SESWebhookPayload) whose Message field
// is a JSON "Received" notification. Unlike Mailgun (form POST + no signature check), SNS
// messages are RSA-signed, so this parser verifies the signature before trusting anything,
// and handles the SNS SubscriptionConfirmation / UnsubscribeConfirmation control messages.
type SESReplyParser struct {
	httpClient *http.Client // SSRF-safe client for cert fetch + subscription confirmation
	certCache  sync.Map     // SigningCertURL -> *rsa.PublicKey
}

// NewSESReplyParser builds the parser with an SSRF-safe HTTP client (used to fetch the SNS
// signing certificate and to GET the SubscribeURL on first subscription).
func NewSESReplyParser(httpClient *http.Client) *SESReplyParser {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &SESReplyParser{httpClient: httpClient}
}

// Source identifies the provider this parser handles.
func (p *SESReplyParser) Source() domain.WebhookSource { return domain.WebhookSourceSES }

// Verify authenticates the SNS message (RSA signature) and handles control messages.
// It returns domain.ErrInboundControlMessage for SubscriptionConfirmation/UnsubscribeConfirmation
// so the caller acks with 200 and ingests nothing. The integration arg is unused for now;
// strict per-integration TopicArn allow-listing arrives with auto-provisioning (the topic
// ARN must be persisted first).
func (p *SESReplyParser) Verify(req *domain.InboundRequest, integration *domain.Integration) error {
	env, err := decodeSNSEnvelope(req)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sesInboundFetchTimeout)
	defer cancel()

	// Authenticate FIRST: a valid signature proves the message genuinely came from AWS SNS,
	// which is what makes the SubscribeURL below safe to fetch.
	if err := p.verifySNSSignature(ctx, env); err != nil {
		return fmt.Errorf("ses inbound: %w", err)
	}

	// Then bind to OUR provisioned topic. A valid SNS signature only proves AWS signed the
	// message — the SNS signing cert covers EVERY topic in EVERY account in the region — so
	// without this check any AWS account could forge a reply for this integration. Fail closed
	// when no topic ARN is bound (inbound never provisioned, or a manual setup that didn't set
	// AmazonSESSettings.InboundTopicARN): an unbound integration cannot authenticate inbound.
	bound := boundInboundTopicARN(integration)
	if bound == "" {
		return fmt.Errorf("ses inbound: no provisioned SNS topic bound to this integration; cannot authenticate inbound mail")
	}
	if env.TopicARN != bound {
		return fmt.Errorf("ses inbound: message TopicArn %q does not match the integration's provisioned topic", env.TopicARN)
	}

	switch env.Type {
	case "SubscriptionConfirmation":
		if err := p.confirmSubscription(ctx, env.SubscribeURL); err != nil {
			return fmt.Errorf("ses inbound: subscription confirm: %w", err)
		}
		return domain.ErrInboundControlMessage
	case "UnsubscribeConfirmation":
		// Acknowledge but do NOT fetch the URL — re-GETting it would re-subscribe us.
		return domain.ErrInboundControlMessage
	case "Notification":
		// Only "Received" notifications carry inbound replies. Any other validly-signed
		// notification from our topic (an operator's SNS-console test publish, an AWS
		// administrative notice) is a control message: ack with 200 so SNS stops retrying,
		// rather than letting Parse 500 and trigger a retry loop.
		if !isReceivedNotification(env.Message) {
			return domain.ErrInboundControlMessage
		}
		return nil
	default:
		return fmt.Errorf("ses inbound: unsupported SNS message type %q", env.Type)
	}
}

// boundInboundTopicARN returns the SNS topic ARN this integration is bound to for inbound
// reply authentication, or "" when none is configured.
func boundInboundTopicARN(integration *domain.Integration) string {
	if integration == nil || integration.EmailProvider.SES == nil {
		return ""
	}
	return integration.EmailProvider.SES.InboundTopicARN
}

// isReceivedNotification reports whether the inner SNS Message is an SES "Received" inbound
// email notification, as opposed to any other notification type published to the same topic.
func isReceivedNotification(message string) bool {
	var probe struct {
		NotificationType string `json:"notificationType"`
	}
	if err := json.Unmarshal([]byte(message), &probe); err != nil {
		return false
	}
	return probe.NotificationType == "Received"
}

// Parse decodes the inner SES "Received" notification into a canonical InboundReply.
func (p *SESReplyParser) Parse(req *domain.InboundRequest) (*domain.InboundReply, error) {
	env, err := decodeSNSEnvelope(req)
	if err != nil {
		return nil, err
	}

	var notif domain.SESReceivedNotification
	if err := json.Unmarshal([]byte(env.Message), &notif); err != nil {
		return nil, fmt.Errorf("ses inbound: decode notification: %w", err)
	}
	if notif.NotificationType != "Received" {
		return nil, fmt.Errorf("ses inbound: unexpected notificationType %q", notif.NotificationType)
	}

	m := notif.Mail
	// mail.headers carries the full header set (incl. In-Reply-To/References); commonHeaders
	// is only a subset. When headers are truncated, fall back to the raw MIME content.
	h := sesHeaderMap(m.Headers)
	if (m.HeadersTruncated || len(m.Headers) == 0) && notif.Content != "" {
		for k, v := range mimeHeadersFromContent(notif.Content) {
			if _, ok := h[k]; !ok {
				h[k] = v
			}
		}
	}

	from := firstSlice(m.CommonHeaders.From)
	from = firstNonEmpty(from, m.Source, h["from"])
	to := firstSlice(m.CommonHeaders.To)
	to = firstNonEmpty(to, firstSlice(m.Destination), h["to"])

	received := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, m.Timestamp); err == nil {
		received = t.UTC()
	}

	return &domain.InboundReply{
		FromEmail:     strings.ToLower(parseReplyAddress(from)),
		ToEmail:       strings.ToLower(parseReplyAddress(to)),
		Subject:       firstNonEmpty(m.CommonHeaders.Subject, h["subject"]),
		MessageID:     stripAngle(firstNonEmpty(m.CommonHeaders.MessageID, h["message-id"])),
		InReplyTo:     firstMessageID(h["in-reply-to"]),
		References:    parseMessageIDList(h["references"]),
		ReceivedAt:    received,
		AutoSubmitted: h["auto-submitted"],
		Precedence:    h["precedence"],
		ContentType:   h["content-type"],
		// Surface the full (lowercased) header set so Classify can detect vendor auto-reply
		// markers (X-Autoreply / X-Autorespond); without this an SES out-of-office that signals
		// only via those headers would be misclassified as a genuine reply and wrongly exit.
		RawHeaders: h,
	}, nil
}

// decodeSNSEnvelope unmarshals the raw SNS POST body (kept on req.Body for JSON providers).
func decodeSNSEnvelope(req *domain.InboundRequest) (*domain.SESWebhookPayload, error) {
	if req == nil || len(req.Body) == 0 {
		return nil, fmt.Errorf("ses inbound: empty request body")
	}
	var env domain.SESWebhookPayload
	if err := json.Unmarshal(req.Body, &env); err != nil {
		return nil, fmt.Errorf("ses inbound: decode SNS envelope: %w", err)
	}
	return &env, nil
}

// verifySNSSignature validates the SNS RSA signature over the canonical string-to-sign.
func (p *SESReplyParser) verifySNSSignature(ctx context.Context, env *domain.SESWebhookPayload) error {
	sts, err := snsStringToSign(env)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}
	pub, err := p.signingKey(ctx, env.SigningCertURL)
	if err != nil {
		return err
	}

	switch env.SignatureVersion {
	case "1":
		sum := sha1.Sum([]byte(sts)) //nolint:gosec // AWS SNS v1 mandates SHA-1.
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA1, sum[:], sig)
	case "2":
		sum := sha256.Sum256([]byte(sts))
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
	default:
		return fmt.Errorf("unsupported SNS signature version %q", env.SignatureVersion)
	}
	if err != nil {
		return fmt.Errorf("SNS signature verification failed: %w", err)
	}
	return nil
}

// snsStringToSign builds the canonical string AWS signs: each included field as
// "Name\nValue\n", in AWS's fixed key order (which differs for control vs notification).
func snsStringToSign(env *domain.SESWebhookPayload) (string, error) {
	var keys []string
	switch env.Type {
	case "Notification":
		keys = []string{"Message", "MessageId", "Subject", "Timestamp", "TopicArn", "Type"}
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		keys = []string{"Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type"}
	default:
		return "", fmt.Errorf("unsupported SNS message type %q", env.Type)
	}
	values := map[string]string{
		"Message":      env.Message,
		"MessageId":    env.MessageID,
		"Subject":      env.Subject,
		"SubscribeURL": env.SubscribeURL,
		"Timestamp":    env.Timestamp,
		"Token":        env.Token,
		"TopicArn":     env.TopicARN,
		"Type":         env.Type,
	}
	var b strings.Builder
	for _, k := range keys {
		v := values[k]
		// Subject is only part of the signed string when present in the message.
		if k == "Subject" && v == "" {
			continue
		}
		b.WriteString(k)
		b.WriteByte('\n')
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// signingKey fetches (and caches) the RSA public key from the SNS signing certificate,
// validating the URL is a genuine AWS SNS host first.
func (p *SESReplyParser) signingKey(ctx context.Context, certURL string) (*rsa.PublicKey, error) {
	if v, ok := p.certCache.Load(certURL); ok {
		return v.(*rsa.PublicKey), nil
	}
	if err := validateSNSURL(certURL); err != nil {
		return nil, fmt.Errorf("signing cert: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch signing cert: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signing cert fetch returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read signing cert: %w", err)
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, fmt.Errorf("signing cert is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse signing cert: %w", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("signing cert public key is not RSA")
	}
	p.certCache.Store(certURL, pub)
	return pub, nil
}

// confirmSubscription GETs the SNS SubscribeURL to activate the HTTPS subscription. The
// caller MUST have verified the signature first.
func (p *SESReplyParser) confirmSubscription(ctx context.Context, subscribeURL string) error {
	if err := validateSNSURL(subscribeURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscribeURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("subscribe URL returned status %d", resp.StatusCode)
	}
	return nil
}

// validateSNSURL ensures the URL is https and a genuine AWS SNS host.
func validateSNSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" || !snsHostRe.MatchString(u.Host) {
		return fmt.Errorf("URL is not a valid AWS SNS host: %q", raw)
	}
	return nil
}

// sesHeaderMap lowercases header names into a map (first occurrence wins).
func sesHeaderMap(headers []domain.SESHeader) map[string]string {
	out := make(map[string]string, len(headers))
	for _, hdr := range headers {
		name := strings.ToLower(strings.TrimSpace(hdr.Name))
		if name == "" {
			continue
		}
		if _, exists := out[name]; exists {
			continue
		}
		out[name] = hdr.Value
	}
	return out
}

// mimeHeadersFromContent best-effort parses the raw MIME header block (base64-decoding the
// content when it decodes cleanly) into a lowercased map. Used only when SNS truncated the
// structured headers.
func mimeHeadersFromContent(content string) map[string]string {
	raw := content
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(content)); err == nil {
		raw = string(decoded)
	}
	out := map[string]string{}
	// Header block ends at the first blank line.
	if idx := strings.Index(raw, "\r\n\r\n"); idx >= 0 {
		raw = raw[:idx]
	} else if idx := strings.Index(raw, "\n\n"); idx >= 0 {
		raw = raw[:idx]
	}
	for _, line := range unfoldHeaderLines(raw) {
		if colon := strings.Index(line, ":"); colon > 0 {
			name := strings.ToLower(strings.TrimSpace(line[:colon]))
			if name != "" {
				if _, ok := out[name]; !ok {
					out[name] = strings.TrimSpace(line[colon+1:])
				}
			}
		}
	}
	return out
}

// unfoldHeaderLines splits a header block into logical lines, joining RFC 5322 folded
// continuations (lines beginning with whitespace) onto the preceding header.
func unfoldHeaderLines(block string) []string {
	var lines []string
	for _, ln := range strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n") {
		if ln == "" {
			continue
		}
		if (ln[0] == ' ' || ln[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += " " + strings.TrimSpace(ln)
			continue
		}
		lines = append(lines, ln)
	}
	return lines
}

// firstSlice returns the first element of a string slice, or "".
func firstSlice(vals []string) string {
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}
