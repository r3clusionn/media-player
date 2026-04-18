package handlers

import (
	"net/http"
	"net/http/httptest"
	"seanime/internal/core"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveSignedClientId(t *testing.T) {
	app := &core.App{ClientIdentitySecret: "test-client-identity-secret"}
	proof := generateClientIdentityProof(app, "client-1")

	assert.Equal(t, "client-1", getSignedClientId(app, "client-1", proof))
	assert.Empty(t, getSignedClientId(app, "client-2", proof))
	assert.Empty(t, getSignedClientId(app, "client-1", "bad-proof"))
}

func TestResolveValidatedClientId(t *testing.T) {
	app := &core.App{ClientIdentitySecret: "test-client-identity-secret"}
	headerProof := generateClientIdentityProof(app, "header-client")
	queryProof := generateClientIdentityProof(app, "query-client")

	t.Run("accepts signed header client id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Header.Set(clientIdHeaderName, "header-client")
		req.Header.Set(clientIdProofHeaderName, headerProof)

		assert.Equal(t, "header-client", resolveValidatedClientID(app, req))
	})

	t.Run("rejects unsigned header client id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		req.Header.Set(clientIdHeaderName, "header-client")

		assert.Empty(t, resolveValidatedClientID(app, req))
	})

	t.Run("accepts signed websocket query client id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events?id=query-client&proof="+queryProof, nil)

		assert.Equal(t, "query-client", resolveValidatedClientID(app, req))
	})
}
