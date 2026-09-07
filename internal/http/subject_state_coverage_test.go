package http

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubjectStateCoverageHandlerRejectsUnknownFieldsAndMethod(t *testing.T) {
	h := &SubjectStateHandler{}
	get := httptest.NewRecorder()
	h.coverage(get, httptest.NewRequest("GET", "/api/subjectStates.coverage", nil))
	if get.Code != 405 {
		t.Fatal(get.Code)
	}
	post := httptest.NewRecorder()
	h.coverage(post, httptest.NewRequest("POST", "/api/subjectStates.coverage", strings.NewReader(`{"workspace_id":"w","unexpected":true}`)))
	if post.Code != 400 {
		t.Fatal(post.Code)
	}
}
