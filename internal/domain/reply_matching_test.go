package domain

import "testing"

func TestRFCMessageIDValueAndHeader(t *testing.T) {
	cases := []struct {
		name       string
		messageID  string
		from       string
		wantValue  string
		wantHeader string
	}{
		{"normal", "abc-123", "hello@example.com", "abc-123@example.com", "<abc-123@example.com>"},
		{"subdomain", "id1", "news@mg.example.com", "id1@mg.example.com", "<id1@mg.example.com>"},
		{"no at sign falls back", "id2", "not-an-address", "id2@notifuse.local", "<id2@notifuse.local>"},
		{"trailing at falls back", "id3", "x@", "id3@notifuse.local", "<id3@notifuse.local>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RFCMessageIDValue(tc.messageID, tc.from); got != tc.wantValue {
				t.Errorf("RFCMessageIDValue = %q, want %q", got, tc.wantValue)
			}
			if got := BuildRFCMessageID(tc.messageID, tc.from); got != tc.wantHeader {
				t.Errorf("BuildRFCMessageID = %q, want %q", got, tc.wantHeader)
			}
		})
	}
}

func TestProviderSetsOwnMessageID(t *testing.T) {
	setOwn := []EmailProviderKind{EmailProviderKindMailgun}
	other := []EmailProviderKind{
		EmailProviderKindSendGrid, EmailProviderKindSparkPost,
		EmailProviderKindMailjet, EmailProviderKindPostmark,
		EmailProviderKindSES, EmailProviderKindSMTP,
	}
	for _, k := range setOwn {
		if !ProviderSetsOwnMessageID(k) {
			t.Errorf("ProviderSetsOwnMessageID(%s) = false, want true", k)
		}
	}
	for _, k := range other {
		if ProviderSetsOwnMessageID(k) {
			t.Errorf("ProviderSetsOwnMessageID(%s) = true, want false (not yet wired)", k)
		}
	}
}

func TestProviderCapturesMessageID(t *testing.T) {
	if !ProviderCapturesMessageID(EmailProviderKindSES) {
		t.Error("ProviderCapturesMessageID(SES) = false, want true")
	}
	// Mailgun sets its own id (not capture); others not wired.
	for _, k := range []EmailProviderKind{
		EmailProviderKindMailgun, EmailProviderKindSendGrid, EmailProviderKindSMTP,
	} {
		if ProviderCapturesMessageID(k) {
			t.Errorf("ProviderCapturesMessageID(%s) = true, want false", k)
		}
	}
	// A set_own and a capture provider must never both claim the same kind.
	if ProviderSetsOwnMessageID(EmailProviderKindSES) {
		t.Error("SES must not be a set_own provider")
	}
}

func TestSESStoredMessageID(t *testing.T) {
	// We store the bare, host-independent local part SES returns (NOT a reconstructed host),
	// because AWS does not pin the Message-ID host.
	if got := SESStoredMessageID("  0000018f-abcd  "); got != "0000018f-abcd" {
		t.Errorf("SESStoredMessageID = %q, want bare trimmed local part", got)
	}
}

func TestSESReplyCandidate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Any amazonses.com host variant resolves to the same local part.
		{"0000018f-abcd@email.amazonses.com", "0000018f-abcd"},
		{"0000018f-abcd@mail.amazonses.com", "0000018f-abcd"},
		{"0000018f-abcd@us-east-1.amazonses.com", "0000018f-abcd"},
		{"0000018f-abcd@EMAIL.AMAZONSES.COM", "0000018f-abcd"}, // case-insensitive
		// Non-SES Message-IDs are left alone (no false local-part match).
		{"uuid-1@mg.example.com", ""},
		{"uuid-1@example.com", ""},
		{"not-amazonses.com.evil.com", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := SESReplyCandidate(c.in); got != c.want {
			t.Errorf("SESReplyCandidate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
