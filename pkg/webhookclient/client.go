// Package webhookclient defines the outbound-network policy shared by Yaoguang webhooks.
package webhookclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/safehttpclient"
)

// AllowPrivateHosts is a deployment-level opt-in, never a workspace-controlled flag.
// An explicit Yaoguang value takes precedence over the compatibility name.
func AllowPrivateHosts() bool {
	value, exists := os.LookupEnv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS")
	if !exists {
		value = os.Getenv("WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS")
	}
	allowed, err := strconv.ParseBool(value)
	return err == nil && allowed
}

// ValidateURL rejects obvious unsafe targets without DNS lookup. The client also
// validates resolved IPs on every connection, including redirects and DNS changes.
func ValidateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL must have a host")
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf("webhook URL must not contain credentials or a fragment")
	}
	if AllowPrivateHosts() {
		return nil
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(host); ip != nil && safehttpclient.IsPrivateIP(ip) {
		return safehttpclient.ErrPrivateIP
	}
	if !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return safehttpclient.ErrPrivateIP
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".localdomain", ".corp", ".intranet"} {
		if strings.HasSuffix(host, suffix) {
			return safehttpclient.ErrPrivateIP
		}
	}
	return nil
}

func New() *http.Client {
	client := safehttpclient.New()
	client.Timeout = 30 * time.Second
	if AllowPrivateHosts() {
		client.Transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	return client
}
