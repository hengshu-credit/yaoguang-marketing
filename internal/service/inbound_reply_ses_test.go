package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SNS v1 uses SHA-1; mirrored from production.
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signSNS signs an envelope's canonical string-to-sign with the given key/version, exactly
// as AWS SNS would, so the parser's verification can be exercised against real RSA math.
func signSNS(t *testing.T, key *rsa.PrivateKey, env *domain.SESWebhookPayload, version string) {
	t.Helper()
	env.SignatureVersion = version
	sts, err := snsStringToSign(env)
	require.NoError(t, err)
	var sig []byte
	switch version {
	case "1":
		sum := sha1.Sum([]byte(sts)) //nolint:gosec
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, sum[:])
	case "2":
		sum := sha256.Sum256([]byte(sts))
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	}
	require.NoError(t, err)
	env.Signature = base64.StdEncoding.EncodeToString(sig)
}

func sesReceivedMessage(from, subject, replyMsgID, inReplyTo, references string) string {
	notif := domain.SESReceivedNotification{
		NotificationType: "Received",
		Mail: domain.SESMail{
			Timestamp:   "2026-06-17T12:00:00Z",
			Source:      from,
			Destination: []string{"hello@example.com"},
			Headers: []domain.SESHeader{
				{Name: "From", Value: from},
				{Name: "To", Value: "hello@example.com"},
				{Name: "Subject", Value: subject},
				{Name: "Message-Id", Value: "<" + replyMsgID + ">"},
				{Name: "In-Reply-To", Value: "<" + inReplyTo + ">"},
				{Name: "References", Value: references},
			},
			CommonHeaders: domain.SESCommonHeaders{
				From: []string{from}, To: []string{"hello@example.com"},
				Subject: subject, MessageID: "<" + replyMsgID + ">",
			},
		},
	}
	b, _ := json.Marshal(notif)
	return string(b)
}

func TestSESReplyParser_Parse(t *testing.T) {
	p := NewSESReplyParser(nil)
	msg := sesReceivedMessage("Jane <jane@example.com>", "Re: Welcome",
		"reply-1@mail.example.com", "orig-abc@email.amazonses.com",
		"<thread-root@x> <orig-abc@email.amazonses.com>")
	body, _ := json.Marshal(domain.SESWebhookPayload{Type: "Notification", Message: msg})

	reply, err := p.Parse(&domain.InboundRequest{Body: body})
	require.NoError(t, err)
	assert.Equal(t, "jane@example.com", reply.FromEmail)
	assert.Equal(t, "hello@example.com", reply.ToEmail)
	assert.Equal(t, "Re: Welcome", reply.Subject)
	assert.Equal(t, "reply-1@mail.example.com", reply.MessageID)
	assert.Equal(t, "orig-abc@email.amazonses.com", reply.InReplyTo, "In-Reply-To drives matching")
	assert.Contains(t, reply.References, "orig-abc@email.amazonses.com")
}

func TestSESReplyParser_Parse_PopulatesRawHeadersForAutoReplyDetection(t *testing.T) {
	// An SES out-of-office that signals ONLY via X-Autoreply (no Auto-Submitted/Precedence)
	// must classify as an auto-responder, not a genuine reply — which requires Parse to surface
	// RawHeaders so Classify can see the vendor header.
	p := NewSESReplyParser(nil)
	notif := domain.SESReceivedNotification{
		NotificationType: "Received",
		Mail: domain.SESMail{
			Source: "ooo@example.com",
			Headers: []domain.SESHeader{
				{Name: "From", Value: "ooo@example.com"},
				{Name: "To", Value: "hello@example.com"},
				{Name: "Subject", Value: "Out of office"},
				{Name: "In-Reply-To", Value: "<orig@email.amazonses.com>"},
				{Name: "X-Autoreply", Value: "yes"},
			},
			CommonHeaders: domain.SESCommonHeaders{From: []string{"ooo@example.com"}},
		},
	}
	msg, _ := json.Marshal(notif)
	body, _ := json.Marshal(domain.SESWebhookPayload{Type: "Notification", Message: string(msg)})

	reply, err := p.Parse(&domain.InboundRequest{Body: body})
	require.NoError(t, err)
	require.NotNil(t, reply.RawHeaders, "RawHeaders must be populated for vendor auto-reply detection")
	assert.Equal(t, domain.ReplyAutoResponder, domain.Classify(reply),
		"X-Autoreply must reach Classify so an SES auto-reply doesn't wrongly exit the journey")
}

func TestSESReplyParser_Parse_HeadersTruncatedFallsBackToContent(t *testing.T) {
	p := NewSESReplyParser(nil)
	rawMIME := "From: jane@example.com\r\nIn-Reply-To: <orig-xyz@email.amazonses.com>\r\nSubject: Re: x\r\n\r\nbody"
	notif := domain.SESReceivedNotification{
		NotificationType: "Received",
		Mail:             domain.SESMail{HeadersTruncated: true, Headers: nil, Source: "jane@example.com"},
		Content:          base64.StdEncoding.EncodeToString([]byte(rawMIME)),
	}
	msg, _ := json.Marshal(notif)
	body, _ := json.Marshal(domain.SESWebhookPayload{Type: "Notification", Message: string(msg)})

	reply, err := p.Parse(&domain.InboundRequest{Body: body})
	require.NoError(t, err)
	assert.Equal(t, "orig-xyz@email.amazonses.com", reply.InReplyTo, "must recover In-Reply-To from raw MIME")
}

func TestSESReplyParser_VerifySignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	for _, ver := range []string{"1", "2"} {
		t.Run("v"+ver, func(t *testing.T) {
			p := NewSESReplyParser(nil)
			certURL := "https://sns.us-east-1.amazonaws.com/cert.pem"
			p.certCache.Store(certURL, &key.PublicKey) // skip the network fetch

			env := &domain.SESWebhookPayload{
				Type: "Notification", MessageID: "m1", TopicARN: "arn:aws:sns:us-east-1:1:t",
				Message: "hello", Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
			}
			signSNS(t, key, env, ver)
			require.NoError(t, p.verifySNSSignature(context.Background(), env), "valid signature must verify")

			env.Message = "tampered" // invalidates the signed digest
			assert.Error(t, p.verifySNSSignature(context.Background(), env), "tampered message must fail")
		})
	}
}

// boundSESIntegration builds an integration whose SES settings are bound to topicARN, so the
// parser's topic-binding check accepts a message carrying that TopicArn.
func boundSESIntegration(topicARN string) *domain.Integration {
	return &domain.Integration{EmailProvider: domain.EmailProvider{
		SES: &domain.AmazonSESSettings{InboundTopicARN: topicARN},
	}}
}

func TestSESReplyParser_Verify_Notification_Passes(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(nil)
	certURL := "https://sns.eu-west-1.amazonaws.com/cert.pem"
	p.certCache.Store(certURL, &key.PublicKey)
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "m1", TopicARN: "arn",
		Message:   `{"notificationType":"Received"}`, // a genuine inbound-email notification
		Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
	}
	signSNS(t, key, env, "2")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn"))
	assert.NoError(t, err, "a signed Received Notification from the bound topic passes")
}

func TestSESReplyParser_Verify_RejectsMismatchedTopic(t *testing.T) {
	// A validly-signed message from a DIFFERENT topic (e.g. an attacker's own AWS account)
	// must be rejected — the SNS signature alone doesn't prove it came from our topic.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(nil)
	certURL := "https://sns.us-east-1.amazonaws.com/cert.pem"
	p.certCache.Store(certURL, &key.PublicKey)
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "m1", TopicARN: "arn:aws:sns:us-east-1:999:attacker-topic",
		Message: `{"notificationType":"Received"}`, Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
	}
	signSNS(t, key, env, "2")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn:aws:sns:us-east-1:111:our-topic"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrInboundControlMessage)
	assert.Contains(t, err.Error(), "does not match the integration's provisioned topic")
}

func TestSESReplyParser_Verify_RejectsUnboundIntegration(t *testing.T) {
	// No provisioned topic ARN bound → fail closed (cannot authenticate), even with a valid sig.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(nil)
	certURL := "https://sns.us-east-1.amazonaws.com/cert.pem"
	p.certCache.Store(certURL, &key.PublicKey)
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "m1", TopicARN: "arn",
		Message: `{"notificationType":"Received"}`, Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
	}
	signSNS(t, key, env, "2")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrInboundControlMessage)
	assert.Contains(t, err.Error(), "no provisioned SNS topic")
}

func TestSESReplyParser_Verify_NonReceivedNotificationIsControlMessage(t *testing.T) {
	// A validly-signed notification from our topic that ISN'T a received email (e.g. an SNS
	// console "Publish test message") must ack as a control message, not 500/retry-loop.
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(nil)
	certURL := "https://sns.us-east-1.amazonaws.com/cert.pem"
	p.certCache.Store(certURL, &key.PublicKey)
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "m1", TopicARN: "arn",
		Message: "arbitrary non-Received test publish", Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
	}
	signSNS(t, key, env, "2")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn"))
	assert.ErrorIs(t, err, domain.ErrInboundControlMessage)
}

func TestSESReplyParser_Verify_RejectsForgedSignature(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	wrong, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(nil)
	certURL := "https://sns.us-east-1.amazonaws.com/cert.pem"
	p.certCache.Store(certURL, &key.PublicKey) // verifier trusts 'key'
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "m1", TopicARN: "arn", Message: "x",
		Timestamp: "2026-06-17T12:00:00Z", SigningCertURL: certURL,
	}
	signSNS(t, wrong, env, "2") // signed with the WRONG key
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrInboundControlMessage, "a forged message must be rejected, not treated as control")
}

func TestSESReplyParser_Verify_SubscriptionConfirmation_ConfirmsAfterVerify(t *testing.T) {
	// Relax the AWS-host guard so the loopback test server is accepted.
	orig := snsHostRe
	snsHostRe = regexp.MustCompile(`.*`)
	defer func() { snsHostRe = orig }()

	confirmed := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		confirmed = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(srv.Client()) // client that trusts the test TLS server
	certURL := srv.URL + "/cert"
	p.certCache.Store(certURL, &key.PublicKey)

	env := &domain.SESWebhookPayload{
		Type: "SubscriptionConfirmation", MessageID: "m1", TopicARN: "arn", Message: "confirm",
		Timestamp: "2026-06-17T12:00:00Z", Token: "tok", SubscribeURL: srv.URL + "/confirm",
		SigningCertURL: certURL,
	}
	signSNS(t, key, env, "1")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn"))
	assert.ErrorIs(t, err, domain.ErrInboundControlMessage)
	assert.True(t, confirmed, "SubscribeURL must be GET-confirmed (only after the signature verified)")
}

func TestSESReplyParser_Verify_UnsubscribeConfirmation_DoesNotGET(t *testing.T) {
	orig := snsHostRe
	snsHostRe = regexp.MustCompile(`.*`)
	defer func() { snsHostRe = orig }()

	getCount := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		getCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewSESReplyParser(srv.Client())
	certURL := srv.URL + "/cert"
	p.certCache.Store(certURL, &key.PublicKey)

	env := &domain.SESWebhookPayload{
		Type: "UnsubscribeConfirmation", MessageID: "m1", TopicARN: "arn", Message: "bye",
		Timestamp: "2026-06-17T12:00:00Z", Token: "tok", SubscribeURL: srv.URL + "/resubscribe",
		SigningCertURL: certURL,
	}
	signSNS(t, key, env, "1")
	body, _ := json.Marshal(env)

	err := p.Verify(&domain.InboundRequest{Body: body}, boundSESIntegration("arn"))
	assert.ErrorIs(t, err, domain.ErrInboundControlMessage)
	assert.Equal(t, 0, getCount, "must NOT GET the URL on unsubscribe (would re-subscribe)")
}

func TestSNSStringToSign_CanonicalOrder(t *testing.T) {
	env := &domain.SESWebhookPayload{
		Type: "Notification", MessageID: "id", Message: "msg",
		Timestamp: "ts", TopicARN: "arn",
	}
	// No Subject set → omitted from the signed string.
	got, err := snsStringToSign(env)
	require.NoError(t, err)
	assert.Equal(t, "Message\nmsg\nMessageId\nid\nTimestamp\nts\nTopicArn\narn\nType\nNotification\n", got)

	env.Subject = "Hello"
	got, err = snsStringToSign(env)
	require.NoError(t, err)
	assert.Contains(t, got, "Subject\nHello\n", "Subject is included when present")
}
