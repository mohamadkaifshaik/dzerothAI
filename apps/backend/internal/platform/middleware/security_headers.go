package middleware

import "net/http"

// SecurityHeaders adds baseline security response headers suitable for a JSON API.
//
// Headers added:
//   - X-Content-Type-Options: nosniff — prevents MIME-type sniffing by browsers.
//   - X-Frame-Options: DENY — prevents the API responses from being embedded in frames.
//   - Referrer-Policy: no-referrer — prevents referrer information leakage.
//
// Headers intentionally NOT added here:
//   - Content-Security-Policy: no browser-rendered HTML surface exists.
//   - Strict-Transport-Security: belongs at the reverse proxy (nginx/Caddy/ALB),
//     not the application server. The proxy terminates TLS.
//   - Permissions-Policy: not relevant for a JSON API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
