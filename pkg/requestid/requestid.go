package requestid

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const Header = "X-Request-ID"

var valid = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// Ensure uses the same bounded identifier in middleware and API response bodies.
func Ensure(r *http.Request) string {
	if r == nil {
		return uuid.NewString()
	}
	id := strings.TrimSpace(r.Header.Get(Header))
	if !valid.MatchString(id) {
		id = uuid.NewString()
	}
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set(Header, id)
	return id
}
