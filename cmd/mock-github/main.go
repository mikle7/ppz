// mock-github — a minimal stand-in for the GitHub OAuth + user
// endpoints, used by the e2e compose stack to test ppz-server's
// /auth/github/* flow without leaving the network.
//
// Endpoints:
//
//	GET  /login/oauth/authorize
//	     ?client_id=…&redirect_uri=…&state=…
//	     302 to <redirect_uri>?code=test_code&state=<state>
//
//	POST /login/oauth/access_token
//	     accepts any client_id+secret+code
//	     returns {"access_token":"test_token","token_type":"bearer"}
//
//	GET  /user
//	     accepts any Bearer token
//	     returns the deterministic test user payload
//
// The single test user is:
//   {"id":99001,"login":"gh-test-user","email":"ghtest@example.com",...}
//
// Listens on $PORT (default :9000). No auth, no state — purely a
// fixture for the test stack.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":9000"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// /login/oauth/authorize → 302 to redirect_uri?code=test_code&state=<state>
	mux.HandleFunc("GET /login/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		redirect := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")
		if redirect == "" {
			http.Error(w, "missing redirect_uri", http.StatusBadRequest)
			return
		}
		u, err := url.Parse(redirect)
		if err != nil {
			http.Error(w, "bad redirect_uri", http.StatusBadRequest)
			return
		}
		q := u.Query()
		q.Set("code", "test_code")
		q.Set("state", state)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})

	// /login/oauth/access_token → token response
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test_token",
			"token_type":   "bearer",
			"scope":        "read:user user:email",
		})
	})

	// /user → deterministic test user
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         99001,
			"login":      "gh-test-user",
			"email":      "ghtest@example.com",
			"avatar_url": "https://avatars.githubusercontent.com/u/99001?v=4",
		})
	})

	log.Printf("mock-github listening on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
