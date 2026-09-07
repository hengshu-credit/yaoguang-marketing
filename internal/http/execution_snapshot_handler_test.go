package http

import (
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecutionSnapshotRoutesRequireAuthentication(t *testing.T) {
	h := NewExecutionSnapshotHandler(nil, func() ([]byte, error) { return []byte("test-secret"), nil }, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	for _, path := range []string{"get", "list", "asset", "observations"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/executions."+path+"?workspace_id=ws", nil))
		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}
func TestExecutionSnapshotMissingHistoryIsExplicit(t *testing.T) {
	h := NewExecutionSnapshotHandler(nil, nil, nil)
	w := httptest.NewRecorder()
	h.writeError(w, httptest.NewRequest(http.MethodGet, "/", nil), domain.ErrExecutionSnapshotNotFound)
	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "historical_snapshot_missing")
}
