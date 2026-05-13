package server

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/clock"
	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/natsauth"
	"github.com/pipescloud/ppz/internal/natsubj"
)

func (s *Server) handleAuthExchange(w http.ResponseWriter, r *http.Request) {
	var req cliproto.AuthExchangeRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, cliproto.New(cliproto.EInvalidAPIKey))
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()

	// Resolve the credential into an org. Two shapes:
	//   ppz_oauth_<…> → OAuth bearer; org defaults to caller's first
	//                    owned org. If req.AccountID is set, validate the
	//                    user is a member of that org and use it.
	//   ppz_<…>       → V1 API key; org = key.AccountID. req.AccountID
	//                    must match (or be empty) — API keys are
	//                    org-scoped at issuance.
	var accountID uuid.UUID
	if strings.HasPrefix(req.APIKey, bearerPrefixOAuth) {
		tok, err := db.LookupBearerToken(ctx, s.Pool, req.APIKey)
		if err != nil {
			writeErr(w, cliproto.New(cliproto.EInvalidAPIKey))
			return
		}
		if req.AccountID != "" {
			parsed, err := uuid.Parse(req.AccountID)
			if err != nil {
				writeErr(w, &cliproto.Error{Code: "E_INVALID_ORG", Message: "org_id is not a valid uuid"})
				return
			}
			if !db.IsMemberOrOwner(ctx, s.Pool, parsed, tok.UserID) {
				writeErr(w, &cliproto.Error{Code: "E_INVALID_ORG", Message: "user is not a member of org"})
				return
			}
			accountID = parsed
		} else {
			ownedOrg, err := db.FirstOwnedAccountFor(ctx, s.Pool, tok.UserID)
			if err != nil {
				writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "no org owned by user"})
				return
			}
			accountID = ownedOrg.ID
		}
	} else {
		key, err := db.LookupAPIKey(ctx, s.Pool, req.APIKey)
		if err != nil {
			writeErr(w, cliproto.New(cliproto.EInvalidAPIKey))
			return
		}
		accountID = key.AccountID
		if req.AccountID != "" && req.AccountID != accountID.String() {
			writeErr(w, &cliproto.Error{Code: "E_INVALID_ORG", Message: "api key is not for this org"})
			return
		}
	}

	// Org name is surfaced in `ppz status` so users see "alpha" instead
	// of a UUID. Looking it up here keeps it on the same round-trip as
	// the auth result; the daemon caches it alongside Credentials.
	org, err := db.GetAccount(ctx, s.Pool, accountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "org lookup: " + err.Error()})
		return
	}

	// Hand back a NATS URL that matches how the client reached us:
	// - host client hit http://localhost:8080 → nats://localhost:4222
	// - in-compose client hit http://ppz-server:8080 → nats://ppz-server:4222
	// PPZ_NATS_PUBLIC_URL (if set) wins over derivation, for ops overrides.
	natsURL := s.NATSURL
	if natsURL == "" {
		host := r.Host // includes port
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		natsURL = "nats://" + host + ":4222"
	}

	// Mint a fresh NATS user JWT for this caller's org — short-lived
	// (5min default). The daemon re-runs /auth/exchange before this
	// expires.
	//
	// Phase 3.5: signed by the org's per-org account signing key
	// (lazily provisioned by AccountPool). Tenant isolation is
	// enforced by NATS at the account boundary; the user JWT just
	// needs broad pub/sub within the account.
	natsUserJWTTTL := 5 * time.Minute
	if s.DevLogin {
		if v := os.Getenv("PPZ_NATS_JWT_TTL"); v != "" {
			if d, perr := time.ParseDuration(v); perr == nil {
				natsUserJWTTTL = d
			}
		}
	}
	oa, err := s.AccountPool.Get(ctx, accountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "provision org account: " + err.Error()})
		return
	}
	natsJWT, natsSeed, err := natsauth.MintUserJWTInAccount(
		oa.AccountPub, oa.SigningKP,
		"ppz-user-"+accountID.String(),
		[]string{">"}, []string{">"},
		clock.Now().Add(natsUserJWTTTL).Unix(),
	)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "mint nats user jwt: " + err.Error()})
		return
	}

	now := clock.Now()
	writeJSON(w, http.StatusOK, cliproto.AuthExchangeReply{
		JWT:          "stub-jwt-not-yet-issued",
		NATSURL:      natsURL,
		AccountID:        accountID.String(),
		AccountName:      org.Name,
		ExpiresAt:    now.Add(natsUserJWTTTL),
		NATSUserJWT:  natsJWT,
		NATSUserSeed: natsSeed,
	})
}

// handleRevokeKey marks an API key revoked. Idempotent: revoking an
// already-revoked key returns 200 (the desired state — revoked — is
// already in place). Missing keys return 404. No auth — same posture
// as the existing /orgs/<id>/keys POST that creates them.
func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid key id", http.StatusBadRequest)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	if err := db.RevokeAPIKey(ctx, s.Pool, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Browser form submit (has Referer) → redirect back so the user
	// sees the updated org page. API clients (curl, no Referer) get
	// a plain 200.
	if ref := r.Referer(); ref != "" {
		http.Redirect(w, r, ref, http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	var req cliproto.CreateSourceRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, cliproto.New(cliproto.EInvalidHandle))
		return
	}
	if err := natsubj.ValidateHandle(req.Handle); err != nil {
		writeErr(w, cliproto.NewInvalidHandle(req.Handle))
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()

	kind := db.SourceKind(req.Kind)
	if kind == "" {
		kind = db.SourceKindMessage
	}
	if kind != db.SourceKindMessage && kind != db.SourceKindPTY {
		writeErr(w, &cliproto.Error{Code: "E_INVALID_KIND", Message: "kind must be 'message' or 'pty'"})
		return
	}

	// Phase 1.5: manifold defaults to '' (root) until the request shape
	// adds an explicit field in Cycle B. The DB column is NOT NULL with
	// default ''.
	src, err := db.InsertSource(ctx, s.Pool, key.AccountID, key.CreatedByUserID, "", req.Handle, kind)
	if err != nil {
		if errors.Is(err, db.ErrHandleTaken) {
			writeErr(w, cliproto.NewSourceTaken(req.Handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	js, err := s.JSFor(ctx, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "org account: " + err.Error()})
		return
	}
	for _, p := range src.Pipes() {
		if err := ensurePipeStream(ctx, js, key.AccountID, src.Handle, p); err != nil {
			writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
			return
		}
	}
	// Surface a representative subject for the new handle. Broadcast is
	// gone (locked decision #16); inbox is the post-Phase-1 default that
	// every handle gets, regardless of kind.
	subject := natsubj.Subject(key.AccountID, src.Handle, "inbox")

	writeJSON(w, http.StatusCreated, cliproto.CreateSourceReply{
		ID:        src.ID.String(),
		Handle:    src.Handle,
		Kind:      string(src.Kind),
		Subject:   subject,
		CreatedAt: src.CreatedAt,
	})
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	ctx, cancel := withTimeout(r)
	defer cancel()
	sources, err := db.ListSourcesForOrg(ctx, s.Pool, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Walk all sources + their user-pipes once to collect every creator
	// id that needs a username for the response, then resolve in one
	// shot. Avoids N+1 user lookups on org pages with many sources.
	pipesBySource := make(map[uuid.UUID][]db.Pipe, len(sources))
	idSet := make(map[uuid.UUID]struct{})
	for _, src := range sources {
		idSet[src.CreatedByUserID] = struct{}{}
		userPipes, err := db.ListPipesForSource(ctx, s.Pool, src.ID)
		if err != nil {
			writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
			return
		}
		pipesBySource[src.ID] = userPipes
		for _, p := range userPipes {
			idSet[p.CreatedByUserID] = struct{}{}
		}
	}
	ids := make([]uuid.UUID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	usernames, err := db.UsernamesByIDs(ctx, s.Pool, ids)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	out := make([]cliproto.Source, 0, len(sources))
	for _, src := range sources {
		userPipes := pipesBySource[src.ID]
		names := make([]string, 0, len(userPipes))
		pipeInfos := make([]cliproto.PipeInfo, 0, len(userPipes))
		for _, p := range userPipes {
			names = append(names, p.Name)
			pipeInfos = append(pipeInfos, cliproto.PipeInfo{
				Pipe:      p.Name,
				CreatedBy: usernames[p.CreatedByUserID],
			})
		}
		out = append(out, cliproto.Source{
			Handle:               src.Handle,
			Kind:                 string(src.Kind),
			Pipes:                names,
			PipeInfos:            pipeInfos,
			CreatedBy:            usernames[src.CreatedByUserID],
			LastBroadcastAt:      src.LastBroadcastAt,
			LastBroadcastPayload: src.LastBroadcastPayload,
		})
	}
	writeJSON(w, http.StatusOK, cliproto.ListSourcesReply{Sources: out})
}

// handleCreatePipe: POST /api/v1/sources/{handle}/pipes
//
// Body: cliproto.PipeCreateRequest. Validates pipe name (regex + reserved
// + not auto-provisioned), inserts the row with retention overrides
// (NULL → server default), provisions the JetStream stream with the
// resolved config, and returns the resolved retention so the caller
// prints exactly what got created.
func (s *Server) handleCreatePipe(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	var req cliproto.PipeCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, cliproto.New(cliproto.EInvalidPipe))
		return
	}
	handle := r.PathValue("handle")
	if err := natsubj.ValidateHandle(handle); err != nil {
		writeErr(w, cliproto.NewInvalidHandle(handle))
		return
	}
	// natsubj.ValidateUserPipeName returns either "invalid pipe name" (regex
	// rejection) or "name is reserved" — keep the distinction so the user
	// sees which one tripped.
	if err := natsubj.ValidateUserPipeName(req.Name); err != nil {
		if err.Error() == "name is reserved" {
			writeErr(w, cliproto.NewInvalidPipeReserved(req.Name))
		} else {
			writeErr(w, cliproto.NewInvalidPipeName(req.Name))
		}
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()

	src, err := db.GetSourceByHandle(ctx, s.Pool, key.AccountID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, cliproto.NewSourceNotFound(handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Phase 1.5: this endpoint is the collared-pipe path (POST
	// /api/v1/sources/{handle}/pipes), so source_id is always set. The
	// pipe inherits the source's manifold (root '' until Cycle B adds
	// explicit manifold support).
	pipe, err := db.InsertPipe(ctx, s.Pool, key.AccountID, src.Manifold, &src.ID, key.CreatedByUserID, req.Name,
		req.TTLSeconds, req.MaxMsgs, req.MaxBytes)
	if err != nil {
		if errors.Is(err, db.ErrPipeNameTaken) {
			writeErr(w, cliproto.NewPipeTaken(req.Name, handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Resolve retention with defaults filled in for any nil fields.
	maxAge := defaultStreamMaxAge
	if pipe.TTLSeconds != nil {
		maxAge = time.Duration(*pipe.TTLSeconds) * time.Second
	}
	maxMsgs := defaultStreamMaxMsgs
	if pipe.MaxMsgs != nil {
		maxMsgs = *pipe.MaxMsgs
	}
	maxBytes := int64(defaultStreamMaxBytes)
	if pipe.MaxBytes != nil {
		maxBytes = *pipe.MaxBytes
	}

	js, err := s.JSFor(ctx, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "org account: " + err.Error()})
		return
	}
	if err := ensurePipeStreamWithRetention(ctx, js, key.AccountID,
		src.Handle, pipe.Name, maxAge, maxMsgs, maxBytes); err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, cliproto.PipeCreateReply{
		Handle:     src.Handle,
		Name:       pipe.Name,
		StreamName: natsubj.StreamName(key.AccountID, src.Handle, pipe.Name),
		TTLSeconds: int(maxAge / time.Second),
		MaxMsgs:    maxMsgs,
		MaxBytes:   maxBytes,
	})
}

// handleCreatePipeFullPath: POST /api/v1/pipes
//
// The Phase 1.5 sourceless (uncollared) pipe creation endpoint. Body is
// cliproto.PipeCreateRequest with SourceHandle == nil and Handle == "".
// Collared pipes still flow through POST /api/v1/sources/{handle}/pipes
// — clients send a collared request there, an uncollared request here.
//
// Splits responsibility cleanly:
//   - Collared shortcut: existing endpoint, source row already known
//   - Uncollared (this): manifold + name, no source row
func (s *Server) handleCreatePipeFullPath(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	var req cliproto.PipeCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, cliproto.New(cliproto.EInvalidPipe))
		return
	}
	if req.Handle != "" || (req.SourceHandle != nil && *req.SourceHandle != "") {
		writeErr(w, &cliproto.Error{Code: "E_INVALID", Message: "collared pipes go to POST /api/v1/sources/{handle}/pipes"})
		return
	}
	if req.Manifold != "" {
		for _, seg := range strings.Split(req.Manifold, ".") {
			if err := natsubj.ValidateHandle(seg); err != nil {
				writeErr(w, &cliproto.Error{Code: "E_INVALID_MANIFOLD", Message: "manifold segment invalid: " + seg})
				return
			}
		}
	}
	if err := natsubj.ValidateUserPipeName(req.Name); err != nil {
		if err.Error() == "name is reserved" {
			writeErr(w, cliproto.NewInvalidPipeReserved(req.Name))
		} else {
			writeErr(w, cliproto.NewInvalidPipeName(req.Name))
		}
		return
	}

	ctx, cancel := withTimeout(r)
	defer cancel()

	pipe, err := db.InsertPipe(ctx, s.Pool, key.AccountID, req.Manifold, nil, key.CreatedByUserID, req.Name,
		req.TTLSeconds, req.MaxMsgs, req.MaxBytes)
	if err != nil {
		if errors.Is(err, db.ErrPipeNameTaken) {
			target := req.Name
			if req.Manifold != "" {
				target = req.Manifold + "." + req.Name
			}
			writeErr(w, cliproto.NewPipeTaken(req.Name, target))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	maxAge := defaultStreamMaxAge
	if pipe.TTLSeconds != nil {
		maxAge = time.Duration(*pipe.TTLSeconds) * time.Second
	}
	maxMsgs := defaultStreamMaxMsgs
	if pipe.MaxMsgs != nil {
		maxMsgs = *pipe.MaxMsgs
	}
	maxBytes := int64(defaultStreamMaxBytes)
	if pipe.MaxBytes != nil {
		maxBytes = *pipe.MaxBytes
	}

	js, err := s.JSFor(ctx, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "account: " + err.Error()})
		return
	}
	if err := ensurePipeStreamPhase15(ctx, js, key.AccountID, req.Manifold, "", pipe.Name, maxAge, maxMsgs, maxBytes); err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, cliproto.PipeCreateReply{
		Handle:     "",
		Manifold:   req.Manifold,
		Name:       pipe.Name,
		StreamName: natsubj.BuildStreamName(key.AccountID, req.Manifold, "", pipe.Name),
		TTLSeconds: int(maxAge / time.Second),
		MaxMsgs:    maxMsgs,
		MaxBytes:   maxBytes,
	})
}

// handleDestroySource: DELETE /api/v1/sources/{handle}
//
// Removes the source row (CASCADE removes pipe rows) then best-effort deletes
// all JetStream streams (auto-provisioned + user-created). Returns 204.
func (s *Server) handleDestroySource(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	handle := r.PathValue("handle")
	if err := natsubj.ValidateHandle(handle); err != nil {
		writeErr(w, cliproto.NewInvalidHandle(handle))
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()

	src, err := db.GetSourceByHandle(ctx, s.Pool, key.AccountID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, cliproto.NewSourceNotFound(handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Snapshot user-created pipe names before CASCADE removes them.
	userPipes, err := db.ListPipesForSource(ctx, s.Pool, src.ID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	if err := db.DeleteSource(ctx, s.Pool, key.AccountID, handle); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, cliproto.NewSourceNotFound(handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	// Stream cleanup is best-effort — the DB row is already gone so orphaned
	// streams are storage waste only, not a correctness problem.
	js, err := s.JSFor(ctx, key.AccountID)
	if err == nil {
		for _, p := range userPipes {
			_ = deletePipeStream(ctx, js, key.AccountID, handle, p.Name)
		}
		for _, p := range src.Pipes() {
			_ = deletePipeStream(ctx, js, key.AccountID, handle, p)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDestroyPipe: DELETE /api/v1/sources/{handle}/pipes/{name}
//
// Removes the row + the JetStream stream. Returns 204 on success.
// Idempotent on missing stream (the row is the source of truth).
func (s *Server) handleDestroyPipe(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	handle := r.PathValue("handle")
	name := r.PathValue("name")
	if err := natsubj.ValidateHandle(handle); err != nil {
		writeErr(w, cliproto.NewInvalidHandle(handle))
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()

	src, err := db.GetSourceByHandle(ctx, s.Pool, key.AccountID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, cliproto.NewSourceNotFound(handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}

	if err := db.DeletePipe(ctx, s.Pool, src.ID, name); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Auto-provisioned pipes (broadcast, inbox, etc.) are
			// JetStream-only — not in the pipes table. Allow destroying
			// them directly via stream deletion rather than returning
			// E_PIPE_NOT_FOUND for pipes the user can see in ppz ls.
			if !src.IsAutoPipe(name) {
				writeErr(w, cliproto.NewPipeNotFound(name, handle))
				return
			}
			// fall through to JetStream cleanup below
		} else {
			writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
			return
		}
	}
	js, err := s.JSFor(ctx, key.AccountID)
	if err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: "org account: " + err.Error()})
		return
	}
	if err := deletePipeStream(ctx, js, key.AccountID, src.Handle, name); err != nil {
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request, key db.APIKey) {
	handle := r.PathValue("handle")
	ctx, cancel := withTimeout(r)
	defer cancel()
	src, err := db.GetSourceByHandle(ctx, s.Pool, key.AccountID, handle)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, cliproto.NewSourceNotFound(handle))
			return
		}
		writeErr(w, &cliproto.Error{Code: "E_INTERNAL", Message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cliproto.Source{
		Handle:               src.Handle,
		Kind:                 string(src.Kind),
		LastBroadcastAt:      src.LastBroadcastAt,
		LastBroadcastPayload: src.LastBroadcastPayload,
	})
}
