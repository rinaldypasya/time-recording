package middleware

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// Logger logs each request with method, path, status, and latency
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s %d %s",
			r.Header.Get("X-Request-ID"),
			r.Method, r.URL.Path, rw.status,
			time.Since(start),
		)
	})
}

// RequestID injects a unique request ID header
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("%016x", rand.Int63())
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(s int) {
	rw.status = s
	rw.ResponseWriter.WriteHeader(s)
}
