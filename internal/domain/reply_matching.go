package domain

import (
	"regexp"
	"strings"
)

// RFCMessageIDValue returns the canonical message-id value WITHOUT angle brackets
// ("messageID@domain", domain taken from the From address). This is what we store
// as message_history.smtp_message_id and what we match against a reply's parsed
// In-Reply-To (also bracket-stripped), so both sides use the identical form.
func RFCMessageIDValue(messageID, fromAddress string) string {
	domain := "notifuse.local"
	if at := strings.LastIndex(fromAddress, "@"); at >= 0 && at+1 < len(fromAddress) {
		domain = fromAddress[at+1:]
	}
	return messageID + "@" + domain
}

// BuildRFCMessageID returns the RFC 5322 Message-ID header value ("<value>") for
// providers that let us set it (set_own). The recipient's reply echoes this in
// In-Reply-To; after bracket-stripping it equals RFCMessageIDValue (the stored
// smtp_message_id), enabling the match.
func BuildRFCMessageID(messageID, fromAddress string) string {
	return "<" + RFCMessageIDValue(messageID, fromAddress) + ">"
}

// ProviderSetsOwnMessageID reports whether Notifuse sets the recipient-visible RFC
// Message-ID itself for this provider (the "set_own" strategy). For these, the
// stored smtp_message_id is BuildRFCMessageID(...). Capture providers (e.g. Mailjet)
// and sender-match providers (SendGrid, SparkPost) are handled separately.
//
// Only providers whose send path actually sets the header are listed here; others
// are added as their send services are updated.
func ProviderSetsOwnMessageID(kind EmailProviderKind) bool {
	switch kind {
	case EmailProviderKindMailgun:
		return true
	default:
		return false
	}
}

// amazonSESMessageIDHost matches the host of an SES-stamped Message-ID. SES overwrites the
// Message-ID with "<localpart@<sub>.amazonses.com>", but the exact subdomain is NOT
// contractually pinned by AWS — public reports show "email.amazonses.com",
// "mail.amazonses.com", and region-qualified variants. So matching keys off the globally
// unique local part (the value SES returns and we store), not the host. Case-insensitive.
var amazonSESMessageIDHost = regexp.MustCompile(`(?i)@[a-z0-9.-]*amazonses\.com$`)

// SESStoredMessageID returns the value to store as message_history.smtp_message_id for a
// captured SES MessageId. SES overwrites the Message-ID header and returns only the
// (globally unique) local part, so we store that bare local part rather than reconstruct a
// host that AWS may change. A reply's In-Reply-To echoes "<localpart@<sub>.amazonses.com>";
// SESReplyCandidate strips the host back to this local part at match time, so matching is
// independent of the exact SES host.
func SESStoredMessageID(returnedMessageID string) string {
	return strings.TrimSpace(returnedMessageID)
}

// SESReplyCandidate returns the host-independent match key for a bracket-stripped
// In-Reply-To / References value: when the value is an "...@<sub>.amazonses.com" address it
// returns the local part (which equals the SES-returned MessageId we stored); otherwise "".
// Used to recover the match when SES's Message-ID host differs from any single assumption.
func SESReplyCandidate(messageID string) string {
	if !amazonSESMessageIDHost.MatchString(messageID) {
		return ""
	}
	if at := strings.IndexByte(messageID, '@'); at > 0 {
		return messageID[:at]
	}
	return ""
}

// ProviderCapturesMessageID reports whether the provider OVERWRITES the Message-ID at send
// time and returns its own (the "capture" strategy): we cannot set our own header, so we
// capture the provider-returned MessageId and store the reconstructed value. Currently SES.
func ProviderCapturesMessageID(kind EmailProviderKind) bool {
	return kind == EmailProviderKindSES
}
