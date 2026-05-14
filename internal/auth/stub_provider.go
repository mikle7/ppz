package auth

import "net/http"

// StubProvider is the OSS-shipped Provider implementation. Used when
// PPZ_SERVER_AUTH_MODE=oauth is set but no real provider has been
// installed (i.e. the deployment isn't pipescloud-hosted). Returns
// HTTP 500 with an explanatory message — better than a silent fall
// through.
type StubProvider struct{}

func (*StubProvider) Authorize(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "oauth provider not configured (PPZ_SERVER_AUTH_MODE=oauth requires an out-of-tree provider implementation)", http.StatusInternalServerError)
}
