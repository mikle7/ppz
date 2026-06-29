package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// TestHandleLogin_ForwardsChosenAccountID — when the user picks an org in
// the OAuth device flow, the CLI hands that org to the daemon as
// LoginRequest.AccountID. handleLogin must forward it as
// AuthExchangeRequest.AccountID so the server mints the NATS JWT in the
// chosen org (the server validates membership there). Dropping it sends
// the caller back to the server's default org — the original bug.
func TestHandleLogin_ForwardsChosenAccountID(t *testing.T) {
	const beta = "00000000-0000-0000-0000-0000000000b2"

	var gotReqAccountID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cliproto.AuthExchangeRequest
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		gotReqAccountID = req.AccountID
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cliproto.AuthExchangeReply{
			NATSURL:      "nats://127.0.0.1:1", // fails fast (DNS) → connect is best-effort
			AccountID:    req.AccountID,
			AccountName:  "beta",
			NATSUserJWT:  "jwt",
			NATSUserSeed: "seed",
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		})
	}))
	defer srv.Close()

	d := &Daemon{
		State:      NewState(t.TempDir()),
		NATSEvents: newNATSEventRing(natsEventRingCap),
		Follows:    newFollowRegistry(),
		Watches:    newWatchRegistry(),
		Heartbeats: NewHeartbeatCache(),
		HTTP:       &http.Client{Timeout: 2 * time.Second},
	}
	t.Cleanup(func() {
		if d.Refresh != nil {
			d.Refresh.Stop()
		}
		if d.NC != nil {
			d.NC.Close()
		}
	})

	srvConn, cliConn := net.Pipe()
	params, _ := json.Marshal(cliproto.LoginRequest{URL: srv.URL, APIKey: "ppz_oauth_test", AccountID: beta})
	go func() {
		d.handleLogin(context.Background(), srvConn, params)
		srvConn.Close()
	}()

	// Drain the IPC reply so handleLogin's writeIPC doesn't block on the
	// unbuffered pipe.
	_ = cliConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(cliConn).ReadBytes('\n')
	if err != nil && err != io.EOF {
		t.Fatalf("reading IPC reply: %v", err)
	}
	var resp struct {
		Result cliproto.LoginReply `json:"result"`
		Error  *cliproto.Error     `json:"error"`
	}
	_ = json.Unmarshal(line, &resp)
	if resp.Error != nil {
		t.Fatalf("login returned error: %v", resp.Error)
	}

	if gotReqAccountID != beta {
		t.Fatalf("handleLogin sent AuthExchangeRequest.AccountID=%q, want %q "+
			"(the org chosen in the device flow); dropping it logs the user into the server default org",
			gotReqAccountID, beta)
	}
}
