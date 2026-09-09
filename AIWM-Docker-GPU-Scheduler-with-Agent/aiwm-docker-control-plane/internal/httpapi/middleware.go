package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request-id"

func requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
		writer.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(request.Context(), requestIDKey, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		capture := &statusWriter{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(capture, request)
		logger.Info("http request",
			"request_id", request.Context().Value(requestIDKey),
			"method", request.Method,
			"path", request.URL.Path,
			"status", capture.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "error", recovered, "stack", string(debug.Stack()))
				writeError(writer, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func cors(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if _, ok := allowed[origin]; ok {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Enrollment-Token, X-Request-ID")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
