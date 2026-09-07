package http

import (
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFunnelHandlerValidationErrorStatus(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/funnels.query", nil)
	writeAnalysisError(w, r, domain.NewValidationError("invalid steps"))
	if w.Code != 400 {
		t.Fatalf("validation must be 400, got %d", w.Code)
	}
}
func TestFunnelHandlerRejectsTrailingJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/funnels.query", strings.NewReader(`{"workspace_id":"w"}{"workspace_id":"other"}`))
	var q domain.FunnelQuery
	if decodeAnalysisRequest(w, r, &q) {
		t.Fatal("ambiguous request must be rejected")
	}
}
