package domain

// NativeBusinessEvent is a reserved occurrence emitted by a supported delivery
// pipeline. The suffixes include the existing email-associated timeline events.
type NativeBusinessEvent struct{ Key, Name string }

func NativeBusinessEvents() []NativeBusinessEvent {
	out := make([]NativeBusinessEvent, 0, 36)
	for _, channel := range []struct{ key, name string }{{"sms", "SMS"}, {"push", "Push"}, {"whatsapp", "WhatsApp"}, {"webhook", "Webhook"}} {
		for _, suffix := range []string{"sent", "delivered", "opened", "clicked", "failed", "bounced", "complained", "unsubscribed", "updated"} {
			out = append(out, NativeBusinessEvent{Key: channel.key + "." + suffix, Name: channel.name + " " + suffix})
		}
	}
	return out
}

func IsNativeBusinessEvent(key string) bool {
	for _, event := range NativeBusinessEvents() {
		if event.Key == key {
			return true
		}
	}
	return false
}
