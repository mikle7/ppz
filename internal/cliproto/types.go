package cliproto

import "time"

// IPC method names. Keep in sync with WIRE.md §7.
const (
	IPCStatus     = "Status"
	IPCLogin      = "Login"
	IPCCreate     = "Create"
	IPCSwitch     = "Switch"
	IPCBroadcast  = "Broadcast"
	IPCList       = "List"
	IPCListWatch  = "ListWatch"
	IPCSubscribe  = "Subscribe"
	IPCRead       = "Read"
	IPCConnect     = "Connect"
	IPCDisconnect  = "Disconnect"
	IPCPipeCreate  = "PipeCreate"
	IPCPipeDestroy = "PipeDestroy"
)

// Source kinds, mirrored from internal/db so non-db callers can use them.
type SourceKind string

const (
	KindMessage SourceKind = "message"
	KindPTY     SourceKind = "pty"
)

// ReadRequest carries the parsed `ppz read` parameters from the CLI to the
// daemon. The daemon streams ReadEvent JSON lines back on the same
// connection; the CLI reads until EOF (or until SIGINT closes the socket).
//
// The `channel` JSON tag is preserved for IPC backward-compat within the
// Phase A rename — it carries a pipe name (sub-bucket). Phase B reorganises
// IPC field names alongside the verb refactor.
type ReadRequest struct {
	Handle    string `json:"handle"`
	Channel   string `json:"channel"`               // pipe name: broadcast / stdin / stdout
	Limit     int    `json:"limit,omitempty"`       // 0 = unlimited; non-zero = tail-N (reread only)
	Skip      int    `json:"skip,omitempty"`        // drop the first N retained messages (reread only)
	SinceMS   int64  `json:"since_ms,omitempty"`    // 0 = no time filter; >0 = only msgs newer than (now − this many ms) (reread only)
	JSON      bool   `json:"json,omitempty"`        // emit envelope as JSON instead of payload text
	Follow    bool   `json:"follow,omitempty"`
	Session   string `json:"session,omitempty"`     // cursor key — defaults to "default" daemon-side
	NoAdvance bool   `json:"no_advance,omitempty"`  // observational reads (terminal view) skip cursor advance
	All       bool   `json:"all,omitempty"`         // forensic mode (`reread`): ignore the cursor (deliver everything) and don't advance it. Implies NoAdvance.
}

// ReadEvent is the wire format of one streamed line in a Read response.
// Exactly one of Message / Error / Meta is set on each event. End-of-stream
// is signaled by the daemon closing the connection.
type ReadEvent struct {
	Message *ReadMessage `json:"msg,omitempty"`
	Error   *Error       `json:"error,omitempty"`
	// Meta is an optional leading event (sent before any Message events)
	// carrying out-of-band stream metadata — currently the source pty's
	// dimensions for `<h>.stdout` reads, sourced from the latest
	// `<h>.stdctrl` resize. Lets the CLI configure its --tty renderer
	// to match the source size before consuming bytes.
	Meta *ReadMeta `json:"meta,omitempty"`
}

// ReadMeta carries leading metadata about the stream. Currently a
// dimension hint for terminal renders; future fields can join here
// (cwd, exit code, last activity ts, etc.) without breaking older
// CLI builds (unknown JSON fields are ignored).
type ReadMeta struct {
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

// ReadMessage is the daemon's serialized form of one envelope. The CLI
// formats this as either bare payload text or a JSON object depending on
// whether --json was passed.
type ReadMessage struct {
	ID        string `json:"id"`
	Handle    string `json:"handle"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// StatusRequest carries the caller's session id so the daemon can return
// the per-session current source. Empty Session normalises to "default".
type StatusRequest struct {
	Session string `json:"session,omitempty"`
}

type StatusReply struct {
	DaemonPID int    `json:"daemon_pid"`
	LoggedIn  bool   `json:"logged_in"`
	URL       string `json:"url,omitempty"`
	KeyPrefix string `json:"key_prefix,omitempty"`
	OrgID     string `json:"org_id,omitempty"`
	OrgName   string `json:"org_name,omitempty"`
	// LoginCheck is the daemon's last verification result against the
	// server. "ok" means a recent server-touching call succeeded;
	// "invalid" means the server returned E_INVALID_API_KEY (key
	// revoked / rotated since login). Empty / "unknown" means we
	// haven't observed yet — status performs an active probe in that
	// case so the user always sees the truth.
	LoginCheck string `json:"login_check,omitempty"`
	Current    string `json:"current,omitempty"`
	// CurrentPath is the daemon-side path to current.json — surfaced so
	// `ppz status`'s env/daemon-disagree warning can point users at the
	// actual file (which lives in the daemon's home, not the CLI's, when
	// they're separate processes).
	CurrentPath string `json:"current_path,omitempty"`
}

// LoginCheck values reported by StatusReply. Constants live in cliproto
// so both daemon (writer) and CLI (reader) share them without import
// cycles. "" is the zero value and means "not applicable" (e.g. no
// credentials stored).
const (
	LoginCheckOK      = "ok"
	LoginCheckInvalid = "invalid"
)

type LoginRequest struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

type LoginReply struct {
	URL       string `json:"url"`
	KeyPrefix string `json:"key_prefix"`
	OrgID     string `json:"org_id"`
}

type CreateRequest struct {
	Handle  string `json:"handle"`
	Kind    string `json:"kind,omitempty"`    // "message" (default) or "pty"
	Session string `json:"session,omitempty"` // sets per-session current after create
}

type CreateReply struct {
	Handle  string   `json:"handle"`
	Subject string   `json:"subject"`
	Pipes   []string `json:"pipes,omitempty"` // pipe names provisioned for this source
}

type SwitchRequest struct {
	Handle  string `json:"handle"`
	Session string `json:"session,omitempty"`
}

type SwitchReply struct {
	Handle string `json:"handle"`
}

// ConnectRequest is the input to `ppz connect <handle>`. The daemon ensures
// the source exists (idempotent — pre-existing source is fine), then sets
// it as `current`.
type ConnectRequest struct {
	Handle  string `json:"handle"`
	Session string `json:"session,omitempty"`
}

type ConnectReply struct {
	Handle string `json:"handle"`
}

// DisconnectRequest carries the session id whose current source should be
// cleared. Empty Session normalises to "default".
type DisconnectRequest struct {
	Session string `json:"session,omitempty"`
}

// DisconnectReply is empty — the only outcome of disconnect is "current"
// being cleared on the daemon side. Returns no fields.
type DisconnectReply struct{}

type BroadcastRequest struct {
	// Optional explicit target. If both empty, daemon publishes to its
	// current source on .broadcast. If Handle is set, publishes to
	// <Handle>.<Channel|"broadcast">. Used by `ppz send` and by
	// `ppz broadcast` when PPZ_CURRENT_HANDLE is exported (e.g. inside
	// a `ppz terminal` child).
	//
	// `channel` JSON tag preserved for IPC backward-compat within Phase A;
	// it carries a pipe name. Phase B reorganises.
	Handle  string `json:"handle,omitempty"`
	Channel string `json:"channel,omitempty"`
	Payload string `json:"payload"`
	// Session keys the per-session current-source fallback when neither
	// Handle nor PPZ_CURRENT_HANDLE is set.
	Session string `json:"session,omitempty"`
}

type BroadcastReply struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Bytes   int    `json:"bytes"`
}

// PipeInfo is per-pipe state surfaced by ppz ls. Total + LastSeq come from
// the JetStream stream's Info; Unread is computed daemon-side from the
// session's cursor file. Preview is the most recent payload truncated to
// 60 chars (UTF-8 safe; ANSI CSI sequences and C0 controls stripped).
type PipeInfo struct {
	Pipe    string     `json:"pipe"`
	Total   uint64     `json:"total"`
	Unread  uint64     `json:"unread"`
	LastSeq uint64     `json:"last_seq,omitempty"`
	LastAt  *time.Time `json:"last_at,omitempty"`
	Preview string     `json:"preview,omitempty"` // truncated to 60 bytes for table view
	Payload string     `json:"payload,omitempty"` // full untruncated payload for `ls --json`
}

type Source struct {
	Handle string `json:"handle"`
	Kind   string `json:"kind,omitempty"`
	// Pipes is the list of user-created pipe names on this source (NOT the
	// auto-provisioned set — derive those from Kind). Set by the server's
	// /api/v1/sources response.
	Pipes     []string   `json:"pipes,omitempty"`
	PipeInfos []PipeInfo `json:"pipe_infos,omitempty"`
	// Legacy broadcast-only summary; kept for the GUI handlers that still
	// read these directly from the postgres-backed sources table.
	LastBroadcastAt      *time.Time `json:"last_broadcast_at,omitempty"`
	LastBroadcastPayload *string    `json:"last_broadcast_payload,omitempty"`
}

type ListRequest struct {
	Session string `json:"session,omitempty"` // cursor key
}

// ListWatchRequest is `ppz ls --watch`. The daemon returns the same shape
// as ListReply, but only after the calling session has at least one
// unread message on a pipe whose handle matches one of Patterns (or any
// handle if Patterns is empty).
//
// Semantics: level-triggered. If unread > 0 already at call time on a
// matching handle, the daemon returns immediately. Otherwise it blocks
// until a new message arrives on a matching subject, then returns.
//
// Patterns are filepath.Match-style globs (`*`, `?`, `[abc]`) matched
// against the handle (not handle.pipe). Multiple patterns OR-combine.
type ListWatchRequest struct {
	Session  string   `json:"session,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
}

type ListReply struct {
	Sources []Source `json:"sources"`
}

// Server HTTP types.

type AuthExchangeRequest struct {
	APIKey string `json:"api_key"`
	// OrgID (Phase 3.5): which org's account to mint a User JWT in.
	// Optional — server defaults to the bearer's primary org (first
	// owned, or first member). Multi-org users specify it explicitly
	// to switch which org their daemon talks to.
	OrgID string `json:"org_id,omitempty"`
}

// OrgInfo is one row in the ListOrgs response — what `ppz orgs ls`
// prints. UUID + display name + role; no NATS-side fields here, this
// endpoint is purely "which orgs am I in".
type OrgInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Role  string `json:"role,omitempty"` // owner / member / viewer / bot (Phase 3.6)
}

type ListOrgsReply struct {
	Orgs []OrgInfo `json:"orgs"`
}

type AuthExchangeReply struct {
	JWT       string    `json:"jwt"`
	NATSURL   string    `json:"nats_url"`
	OrgID     string    `json:"org_id"`
	OrgName   string    `json:"org_name"`
	ExpiresAt time.Time `json:"expires_at"`

	// Auth V2 Phase 3 — short-lived NATS user credentials. The daemon
	// uses NATSUserJWT + NATSUserSeed in nats.UserJWT(...) when
	// connecting to the NATS server URL. Re-fetch before ExpiresAt
	// (currently 5min) by re-running /auth/exchange with the same
	// bearer.
	NATSUserJWT  string `json:"nats_user_jwt"`
	NATSUserSeed string `json:"nats_user_seed"`
}

type CreateSourceRequest struct {
	Handle string `json:"handle"`
	Kind   string `json:"kind,omitempty"` // "message" (default) or "pty"
}

type CreateSourceReply struct {
	ID        string    `json:"id"`
	Handle    string    `json:"handle"`
	Kind      string    `json:"kind"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
}

type ListSourcesReply struct {
	Sources []Source `json:"sources"`
}

// PipeCreateRequest is the input to `ppz pipe create <name>` — and the body
// of POST /api/v1/sources/{handle}/pipes. Retention overrides are pointers
// so "absent" (= use default) is distinguishable from "explicitly zero".
type PipeCreateRequest struct {
	Handle     string `json:"handle"`
	Name       string `json:"name"`
	TTLSeconds *int   `json:"ttl_seconds,omitempty"`
	MaxMsgs    *int   `json:"max_msgs,omitempty"`
	MaxBytes   *int64 `json:"max_bytes,omitempty"`
}

// PipeCreateReply mirrors the server's resolved retention (after defaults
// are filled in) so the CLI prints exactly what was provisioned.
type PipeCreateReply struct {
	Handle      string `json:"handle"`
	Name        string `json:"name"`
	StreamName  string `json:"stream_name"`
	TTLSeconds  int    `json:"ttl_seconds"`
	MaxMsgs     int    `json:"max_msgs"`
	MaxBytes    int64  `json:"max_bytes"`
}

type PipeDestroyRequest struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
}

type PipeDestroyReply struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
}

// HTTPError is the body shape of a non-2xx HTTP response.
type HTTPError struct {
	Error Error `json:"error"`
}
