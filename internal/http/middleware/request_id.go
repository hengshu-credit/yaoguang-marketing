package middleware

import (
	"net/http"

	"github.com/hengshu-credit/yaoguang-marketing/pkg/logger"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/requestid"
)

func RequestIDMiddleware(next http.Handler, log logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestid.Ensure(r)
		w.Header().Set(requestid.Header, id)
		if log != nil {
			log.WithFields(map[string]interface{}{"request_id": id, "method": r.Method, "path": r.URL.Path}).Debug("HTTP request")
		}
		next.ServeHTTP(w, r)
	})
}
