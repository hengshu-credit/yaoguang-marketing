package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/requestid"
	"github.com/stretchr/testify/require"
)

func TestRequestIDMiddlewareIncludesErrorsAndPreflight(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		for _, supplied := range []string{"", "customer-request:123", "unsafe\nvalue", strings.Repeat("x", 129)} {
			req := httptest.NewRequest(method, "/api/customers.get", nil)
			req.Header.Set(requestid.Header, supplied)
			response := httptest.NewRecorder()
			handler := RequestIDMiddleware(CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, r.Header.Get(requestid.Header), requestid.Ensure(r))
				w.WriteHeader(http.StatusForbidden)
			})), nil)
			handler.ServeHTTP(response, req)
			id := response.Header().Get(requestid.Header)
			require.NotEmpty(t, id)
			require.NotContains(t, id, "\n")
			require.LessOrEqual(t, len(id), 128)
			if supplied == "customer-request:123" {
				require.Equal(t, supplied, id)
			}
			require.Equal(t, "X-Request-ID", response.Header().Get("Access-Control-Expose-Headers"))
		}
	}
}
