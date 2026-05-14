// Package auth defines the OSS Provider interface for delegated
// authentication (auth_mode=oauth). The OSS binary ships a stub
// implementation; pipescloud implements the real provider out-of-tree
// against this interface. See pipes-internal/docs/PHASE-2-IMPLEMENTATION-PLAN.md
// Cycle D for the full contract.
package auth

import "net/http"

// Provider authenticates a browser user via an out-of-tree
// authorization flow (e.g. GitHub OAuth, SAML). The OSS server's
// /login handler calls Authorize() under auth_mode=oauth; the
// provider redirects to its upstream authorize endpoint and later
// hands control back via a callback route the provider owns.
//
// The session-cookie contract is the same across all three auth
// modes — the provider is responsible for setting it once the user
// is identified.
type Provider interface {
	// Authorize handles the initial GET /login request. Implementations
	// typically write a 302 redirect to an upstream authorize endpoint
	// (carrying state). The OSS stub returns a 500 with a "provider
	// not configured" message so deployments that set
	// PPZ_SERVER_AUTH_MODE=oauth without installing a real provider
	// fail loudly rather than silently.
	Authorize(w http.ResponseWriter, r *http.Request)
}
