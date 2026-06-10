package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Middleware struct {
	apiKeys    map[string]*APIKeyInfo
	allowedOrigins []string
	logger     *slog.Logger
}

type APIKeyInfo struct {
	Name        string
	Permissions map[string]bool
}

func NewMiddleware(apiKeys map[string]*APIKeyInfo, allowedOrigins []string, logger *slog.Logger) *Middleware {
	return &Middleware{
		apiKeys:        apiKeys,
		allowedOrigins: allowedOrigins,
		logger:         logger,
	}
}

func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, allowed := range m.allowedOrigins {
			if origin == allowed || allowed == "*" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/f/") || r.URL.Path == "/healthz" || r.URL.Path == "/" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, `{"error":"invalid authorization format, use Bearer <key>"}`, http.StatusUnauthorized)
			return
		}

		hash := sha256.Sum256([]byte(token))
		hashStr := hex.EncodeToString(hash[:])

		keyInfo, ok := m.apiKeys[hashStr]
		if !ok {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}

		requiredPerm := methodToPermission(r.Method)
		if !keyInfo.Permissions[requiredPerm] && !keyInfo.Permissions["admin"] {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		m.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func methodToPermission(method string) string {
	switch method {
	case "GET":
		return "read"
	case "POST", "PUT":
		return "write"
	case "DELETE":
		return "delete"
	default:
		return "admin"
	}
}

func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func BuildAPIKeyMap(keys []struct {
	Name        string
	Key         string
	Permissions []string
}) map[string]*APIKeyInfo {
	m := make(map[string]*APIKeyInfo)
	for _, k := range keys {
		hash := HashAPIKey(k.Key)
		perms := make(map[string]bool)
		for _, p := range k.Permissions {
			perms[p] = true
		}
		m[hash] = &APIKeyInfo{
			Name:        k.Name,
			Permissions: perms,
		}
	}
	return m
}
