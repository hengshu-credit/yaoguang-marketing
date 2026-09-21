package webhookclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/safehttpclient"
	"github.com/stretchr/testify/require"
)

func TestValidateURL(t *testing.T) {
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "false")
	for _, target := range []string{
		"http://127.0.0.1/", "http://localhost./", "http://[::1]/", "http://[::ffff:127.0.0.1]/",
		"http://169.254.169.254/latest/meta-data", "https://10.2.3.4/", "http://100.64.0.1/",
		"https://service.cluster.local/", "http://metadata.google.internal/", "http://postgres/",
		"https://user:password@example.com/", "https://example.com/#fragment", "file:///etc/passwd",
	} {
		t.Run(target, func(t *testing.T) { require.Error(t, ValidateURL(target)) })
	}
	require.NoError(t, ValidateURL("https://hooks.example.com/path?token=opaque"))
}

func TestPrivateDestinationRequiresExplicitDeploymentOptIn(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv("WEBHOOK_DELIVERY_ALLOW_PRIVATE_HOSTS", "true")
	for _, setting := range []string{"false", "invalid", ""} {
		t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", setting)
		_, err := New().Get(server.URL)
		require.True(t, errors.Is(err, safehttpclient.ErrPrivateIP), "%v", err)
	}
	require.Zero(t, requests)
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "true")
	require.NoError(t, ValidateURL(server.URL))
	response, err := New().Get(server.URL)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 1, requests)
}

func TestRedirectRevalidatesDestination(t *testing.T) {
	t.Setenv("YAOGUANG_WEBHOOK_ALLOW_PRIVATE_HOSTS", "false")
	client := New()
	// Serve the first hop in memory; the redirected connection must use the
	// real protected transport, without relying on an external DNS/network.
	transport := client.Transport
	client.Transport = redirectTransport{next: transport}
	_, err := client.Get("https://public.example.com/hook")
	require.ErrorIs(t, err, safehttpclient.ErrPrivateIP)
}

type redirectTransport struct {
	next http.RoundTripper
}

func (t redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "public.example.com" {
		return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": {"http://127.0.0.1/secret"}}, Body: http.NoBody, Request: r}, nil
	}
	return t.next.RoundTrip(r)
}
