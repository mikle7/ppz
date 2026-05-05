package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// org-management IPC handlers — Phase 4 (multi-org support).
//
//	ppz org list                    → IPCOrgList     (proxy GET  /api/v1/orgs)
//	ppz org switch <name>           → IPCOrgSwitch   (re-run auth/exchange w/ org_id)
//	ppz org create <name>           → IPCOrgCreate   (proxy POST /api/v1/orgs)
//	ppz org invite <username>       → IPCOrgInvite   (proxy POST /api/v1/orgs/<current>/invites)

func (d *Daemon) handleOrgList(ctx context.Context, conn net.Conn, _ json.RawMessage) {
	var reply cliproto.ListOrgsReply
	if e := d.callServer(ctx, "GET", "/api/v1/orgs", nil, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	// Annotate the org the daemon is currently bound to. The server
	// doesn't know — that's daemon-side state set by login / org
	// switch — so we stamp it here before handing back to the CLI.
	if cur := d.State.OrgID(); cur != "" {
		for i := range reply.Orgs {
			if reply.Orgs[i].ID == cur {
				reply.Orgs[i].Current = true
			}
		}
	}
	writeIPC(conn, reply)
}

func (d *Daemon) handleOrgCreate(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.OrgCreateRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	var reply cliproto.CreateOrgReply
	if e := d.callServer(ctx, "POST", "/api/v1/orgs", cliproto.CreateOrgRequest{Name: req.Name}, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

func (d *Daemon) handleOrgInvite(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.OrgInviteRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	orgName := d.State.OrgName()
	if orgName == "" {
		writeIPCErr(conn, &cliproto.Error{Code: "E_NO_CURRENT_ORG", Message: "not logged in or no current org"})
		return
	}
	var reply cliproto.CreateInviteReply
	if e := d.callServer(ctx, "POST", "/api/v1/orgs/"+orgName+"/invites",
		cliproto.CreateInviteRequest{Username: req.Username}, &reply); e != nil {
		writeIPCErr(conn, e)
		return
	}
	writeIPC(conn, reply)
}

// handleOrgSwitch re-runs /api/v1/auth/exchange with the new org_id,
// persists the resulting credentials (including the new NATS user
// JWT/seed), restarts the refresh loop, and reconnects NATS so
// subsequent broadcast/list calls hit the new org's account.
func (d *Daemon) handleOrgSwitch(ctx context.Context, conn net.Conn, params json.RawMessage) {
	var req cliproto.OrgSwitchRequest
	if err := json.Unmarshal(params, &req); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}
	if req.Name == "" {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INVALID_ORG", Message: "name is required"})
		return
	}

	// Look up the target org's id by listing what the caller has
	// access to and matching by name. This both resolves the slug and
	// pre-validates membership before the auth/exchange call.
	var orgs cliproto.ListOrgsReply
	if e := d.callServer(ctx, "GET", "/api/v1/orgs", nil, &orgs); e != nil {
		writeIPCErr(conn, e)
		return
	}
	var targetID string
	for _, o := range orgs.Orgs {
		if o.Name == req.Name {
			targetID = o.ID
			break
		}
	}
	if targetID == "" {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INVALID_ORG", Message: "no such org or not a member: " + req.Name})
		return
	}

	creds, ok := d.State.Credentials()
	if !ok {
		writeIPCErr(conn, cliproto.New(cliproto.ENotLoggedIn))
		return
	}
	body, _ := json.Marshal(cliproto.AuthExchangeRequest{APIKey: creds.APIKey, OrgID: targetID})
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", creds.URL+"/api/v1/auth/exchange", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(httpReq)
	if err != nil {
		writeIPCErr(conn, cliproto.New(cliproto.EServerUnreachable))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		writeIPCErr(conn, cliproto.New(cliproto.EInvalidAPIKey))
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeIPCErr(conn, &cliproto.Error{Code: "E_HTTP", Message: resp.Status})
		return
	}
	var ex cliproto.AuthExchangeReply
	if err := json.NewDecoder(resp.Body).Decode(&ex); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_PROTOCOL", Message: err.Error()})
		return
	}

	creds.NATSUserJWT = ex.NATSUserJWT
	creds.NATSUserSeed = ex.NATSUserSeed
	if err := d.State.SetLogin(*creds, ex.OrgID, ex.OrgName, keyPrefix(creds.APIKey)); err != nil {
		writeIPCErr(conn, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Reconnect NATS to the (potentially new) account.
	natsURL := ex.NATSURL
	if v := os.Getenv("PPZ_NATS_URL"); v != "" {
		natsURL = v
	}
	d.NATSURL = natsURL
	if d.NC != nil {
		d.NC.Close()
		d.NC = nil
	}
	d.startRefreshLoop(ex.OrgID, ex.NATSUserJWT, ex.NATSUserSeed, ex.ExpiresAt.Unix())
	if nc, err := connectNATSWithRefresh(natsURL, d.Refresh); err == nil {
		d.NC = nc
	}

	writeIPC(conn, cliproto.OrgSwitchReply{OrgID: ex.OrgID, OrgName: ex.OrgName})
}

