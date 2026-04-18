package handlers

import (
	"net/http"
	"seanime/internal/core"
	"strings"
)

const (
	clientIdHeaderName      = "X-Seanime-Client-Id"
	clientIdProofHeaderName = "X-Seanime-Client-Id-Proof"
	clientIdCookieName      = "Seanime-Client-Id"
	clientIdQueryParam      = "id"
	clientIdProofQueryParam = "proof"
	clientIdTokenPrefix     = "client-id:"
)

func formatClientIdTokenSubject(clientID string) string {
	return clientIdTokenPrefix + strings.TrimSpace(clientID)
}

func generateClientIdentityProof(app *core.App, clientID string) string {
	clientID = strings.TrimSpace(clientID)
	if app == nil || clientID == "" {
		return ""
	}

	proof, err := app.GetClientIdentityHMACAuth().GenerateToken(formatClientIdTokenSubject(clientID))
	if err != nil {
		return ""
	}

	return proof
}

func getSignedClientId(app *core.App, claimedId string, proof string) string {
	claimedId = strings.TrimSpace(claimedId)
	proof = strings.TrimSpace(proof)
	if app == nil || claimedId == "" || proof == "" {
		return ""
	}

	if _, err := app.GetClientIdentityHMACAuth().ValidateToken(proof, formatClientIdTokenSubject(claimedId)); err != nil {
		return ""
	}

	return claimedId
}

func resolveValidatedClientID(app *core.App, req *http.Request) string {
	if req == nil {
		return ""
	}

	if clientID := getSignedClientId(app, req.Header.Get(clientIdHeaderName), req.Header.Get(clientIdProofHeaderName)); clientID != "" {
		return clientID
	}

	if req.URL != nil && req.URL.Path == "/events" {
		if clientID := getSignedClientId(app, req.URL.Query().Get(clientIdQueryParam), req.URL.Query().Get(clientIdProofQueryParam)); clientID != "" {
			return clientID
		}
	}

	return ""
}

func setClientIdentityHeaders(headers http.Header, app *core.App, clientID string) {
	clientID = strings.TrimSpace(clientID)
	if headers == nil || clientID == "" {
		return
	}

	headers.Set(clientIdHeaderName, clientID)
	if proof := generateClientIdentityProof(app, clientID); proof != "" {
		headers.Set(clientIdProofHeaderName, proof)
	}
}
