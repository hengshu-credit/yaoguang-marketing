package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

// syntheticReplyMessageID derives a stable dedup key for an inbound reply that carries no
// Message-Id of its own, from its content (sender + threading headers + subject). A
// provider retry of the same reply hashes identically, so the (message_id) dedup index
// still catches it; two genuinely distinct headerless replies on the same thread may
// collide, which only drops a duplicate timeline entry — never a wrong exit.
func syntheticReplyMessageID(reply *domain.InboundReply) string {
	sum := sha256.Sum256([]byte(reply.FromEmail + "|" + reply.InReplyTo + "|" +
		strings.Join(reply.References, ",") + "|" + reply.Subject))
	return "synthetic-" + hex.EncodeToString(sum[:16]) + "@inbound.notifuse"
}

// parseReplyAddress returns the bare email address from a header value that may be
// "Name <addr@example.com>" or a plain address; returns "" for empty input. The
// caller lowercases where needed.
func parseReplyAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(value); err == nil {
		return addr.Address
	}
	return value
}

// stripAngle removes surrounding angle brackets and whitespace from a message-id.
func stripAngle(s string) string {
	return strings.Trim(strings.TrimSpace(s), "<>")
}

// firstNonEmpty returns the first value that is non-empty after trimming.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// firstMessageID extracts the first message-id from a header that may contain
// several (whitespace-separated), with angle brackets stripped.
func firstMessageID(value string) string {
	ids := parseMessageIDList(value)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// parseMessageIDList splits a References / In-Reply-To header into individual
// message-ids (brackets stripped, empties discarded).
func parseMessageIDList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if id := stripAngle(f); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// parseMailgunHeaders parses Mailgun's "message-headers" field — a JSON list of
// [name, value] pairs — into a map keyed by lowercased header name (first wins).
func parseMailgunHeaders(jsonStr string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(jsonStr) == "" {
		return out
	}
	var pairs [][]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &pairs); err != nil {
		return out
	}
	for _, pair := range pairs {
		if len(pair) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(pair[0])))
		if name == "" {
			continue
		}
		if _, exists := out[name]; exists {
			continue // first occurrence wins
		}
		out[name] = fmt.Sprint(pair[1])
	}
	return out
}
