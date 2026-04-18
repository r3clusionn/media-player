package handlers

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

var errTooManyRequests = errors.New("too many requests")
var errTooManyAuthenticationAttempts = errors.New("too many authentication attempts")

type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

type rateLimitStore struct {
	mu      sync.Mutex
	windows map[string]*rateLimitWindow
}

func newRateLimitStore() *rateLimitStore {
	return &rateLimitStore{windows: make(map[string]*rateLimitWindow)}
}

func (s *rateLimitStore) allow(key string, limit int, window time.Duration) bool {
	if s == nil {
		return true
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for existingKey, entry := range s.windows {
		if now.After(entry.resetAt) {
			delete(s.windows, existingKey)
		}
	}

	entry, ok := s.windows[key]
	if !ok || now.After(entry.resetAt) {
		s.windows[key] = &rateLimitWindow{count: 1, resetAt: now.Add(window)}
		return true
	}

	if entry.count >= limit {
		return false
	}

	entry.count++
	return true
}

func (s *rateLimitStore) reset(key string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.windows, key)
}

var (
	authFailureRateLimits         = newRateLimitStore()
	websocketUpgradeRateLimits    = newRateLimitStore()
	controlPlaneMutationLimits    = newRateLimitStore()
	authFailureWindow             = 5 * time.Minute
	websocketUpgradeWindow        = time.Minute
	controlPlaneMutationWindow    = time.Minute
	maxAuthFailuresPerWindow      = 10
	maxWebsocketAttemptsPerWindow = 40
	maxMutationsPerWindow         = 90
)

func authFailureRateLimitKey(req *http.Request) string {
	return "auth:" + requestClientIP(req)
}

func websocketUpgradeRateLimitKey(req *http.Request) string {
	return "ws:" + requestClientIP(req)
}

func controlPlaneMutationRateLimitKey(req *http.Request) string {
	return "mutate:" + requestClientIP(req)
}

func shouldRateLimitMutation(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}

	if !strings.HasPrefix(req.URL.Path, "/api/") {
		return false
	}

	switch req.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func getBodyLimit(path string) int64 {
	switch {
	case path == "/api/v1/library/local-files/import":
		return 8 << 20
	case path == "/api/v1/extensions/external/edit-payload":
		return 4 << 20
	case path == "/api/v1/extensions/playground/run":
		return 4 << 20
	default:
		return 2 << 20
	}
}

func (h *Handler) controlPlaneBodyLimitMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if req == nil || req.URL == nil || req.Body == nil || !strings.HasPrefix(req.URL.Path, "/api/") {
			return next(c)
		}

		switch req.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		default:
			return next(c)
		}

		limit := getBodyLimit(req.URL.Path)
		if req.ContentLength > limit {
			return h.RespondWithStatusError(c, http.StatusRequestEntityTooLarge, errors.New("request body too large"))
		}

		req.Body = http.MaxBytesReader(c.Response(), req.Body, limit)
		return next(c)
	}
}

func (h *Handler) controlPlaneMutationRateLimitMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if !shouldRateLimitMutation(req) {
			return next(c)
		}

		key := controlPlaneMutationRateLimitKey(req)
		if !controlPlaneMutationLimits.allow(key, maxMutationsPerWindow, controlPlaneMutationWindow) {
			return h.RespondWithStatusError(c, http.StatusTooManyRequests, errTooManyRequests)
		}

		return next(c)
	}
}
