package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pipescloud/ppz/internal/cliproto"
	"github.com/pipescloud/ppz/internal/db"
	"github.com/pipescloud/ppz/internal/envelope"
	"github.com/pipescloud/ppz/internal/natsubj"
)

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// resolveOrg accepts either a UUID or the org's unique name (slug). UUID
// parse is tried first; on failure we fall back to a name lookup. A
// UUID-shaped name would always be treated as an ID — names in practice are
// short alphanumerics, so this is fine.
func resolveOrg(ctx context.Context, pool *db.Pool, idOrSlug string) (db.Organisation, error) {
	if id, err := uuid.Parse(idOrSlug); err == nil {
		return db.GetOrganisation(ctx, pool, id)
	}
	return db.GetOrganisationByName(ctx, pool, idOrSlug)
}

// handleGUILanding renders the marketing landing at `/`. Logo + tagline
// + two animated terminal demos. No DB calls — fully static. The
// operator-facing org index lives at `/dashboard`.
func (s *Server) handleGUILanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "landing.html", nil); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleGUIIndex(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r)
	defer cancel()
	orgs, err := db.ListOrganisationsForUser(ctx, s.Pool, UserIDFromCtx(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "index.html", map[string]any{"Orgs": orgs}); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleGUICreateOrg(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", 400)
		return
	}
	// Default owner to the currently-authenticated user — otherwise
	// the creator wouldn't see their own org on /dashboard (which is
	// scoped by ownership/membership). An explicit owner_user_id form
	// field still wins, for the rare case a tool needs to assign on
	// behalf of someone else.
	ownerID := UserIDFromCtx(r.Context())
	if v := strings.TrimSpace(r.FormValue("owner_user_id")); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			http.Error(w, "owner_user_id is not a valid uuid", 400)
			return
		}
		ownerID = parsed
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	if _, err := db.InsertOrganisation(ctx, s.Pool, name, ownerID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Browser submit (Referer set) → redirect back; API client (no
	// Referer, e.g. test harness via curl -L) → plain 200 to avoid
	// the curl-follows-with-POST → 405 dance. See browserSubmit.
	browserSubmit(w, r)
}

// handleGUIOrgRedirect is the bare `/orgs/{id}` entry point. The page
// is split into three tabs (pipes / users / keys); we 303 the visitor
// to the default tab so deep-linking + refresh land them on the same
// place. Pipes wins as default — it's the operator's most-used view.
func (s *Server) handleGUIOrgRedirect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r)
	defer cancel()
	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", 404)
		return
	}
	http.Redirect(w, r, "/orgs/"+org.Name+"/pipes", http.StatusSeeOther)
}

// orgTabs lists the tab keys the GUI knows about. The router registers
// one route per entry; the handler validates against this set so only
// valid tabs render.
var orgTabs = map[string]bool{"pipes": true, "users": true, "keys": true}

// handleGUIOrgTab dispatches the org page for a given tab. The tab
// drives which section the template renders, but the same shared
// header + sub-nav always appears so the user can switch.
func (s *Server) handleGUIOrgTab(w http.ResponseWriter, r *http.Request) {
	tab := r.PathValue("tab")
	if !orgTabs[tab] {
		http.Error(w, "unknown tab", http.StatusNotFound)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", 404)
		return
	}
	keys, err := db.ListAPIKeysForOrg(ctx, s.Pool, org.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sources, err := db.ListSourcesForOrg(ctx, s.Pool, org.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type sourceRow struct {
		Handle         string
		Pipe           string // broadcast / stdin / stdout / user-created
		PipeLink       string // /orgs/<slug>/sources/<handle>/pipes/<pipe>
		TerminalLink   string // /orgs/<slug>/sources/<handle>/terminal — only set on stdout rows that have data (a shared terminal)
		LastMessageAt  string // relative duration ("5 seconds ago" / "just now") or ""
		PayloadDisplay string // truncated to 60 chars or ""
		HasLastMessage bool
	}
	// One row per (source, pipe). The pipe set is the union of:
	//   - auto-provisioned pipes derived from kind (`src.Pipes()`)
	//   - user-created pipes in the `pipes` table
	// "Last message" + payload come from JetStream — per-pipe, not just the
	// broadcast mirror in postgres — so user pipes (and stdin/stdout for
	// pty sources) get the same freshness signal as broadcast.
	now := time.Now()
	rows := make([]sourceRow, 0, len(sources))
	for _, src := range sources {
		pipeSet := map[string]struct{}{}
		for _, p := range src.Pipes() {
			pipeSet[p] = struct{}{}
		}
		userPipes, err := db.ListPipesForSource(ctx, s.Pool, src.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		for _, up := range userPipes {
			pipeSet[up.Name] = struct{}{}
		}
		pipes := make([]string, 0, len(pipeSet))
		for p := range pipeSet {
			pipes = append(pipes, p)
		}
		sort.Strings(pipes)
		// Phase 3.5: per-org JS. If account not provisioned yet,
		// rows still render without last-message data.
		js, _ := s.JSFor(ctx, org.ID)
		for _, p := range pipes {
			row := sourceRow{
				Handle:   src.Handle,
				Pipe:     p,
				PipeLink: fmt.Sprintf("/orgs/%s/sources/%s/pipes/%s", org.Name, src.Handle, p),
			}
			streamName := natsubj.StreamName(org.ID, src.Handle, p)
			if js == nil {
				rows = append(rows, row)
				continue
			}
			if stream, err := js.Stream(ctx, streamName); err == nil {
				if info, ierr := stream.Info(ctx); ierr == nil && info.State.Msgs > 0 {
					if !info.State.LastTime.IsZero() {
						row.LastMessageAt = cliproto.RelativeTime(info.State.LastTime, now)
						row.HasLastMessage = true
					}
					if msg, mErr := stream.GetMsg(ctx, info.State.LastSeq); mErr == nil {
						if env, eErr := envelope.Unmarshal(msg.Data); eErr == nil {
							row.PayloadDisplay = cliproto.TruncatePayload(env.Payload)
						}
					}
				}
			}
			// Surface the live-terminal link on stdout rows that actually
			// have data — that's the signal someone's shared a terminal
			// here (vs. a never-used auto-provisioned pipe).
			if p == "stdout" && row.HasLastMessage {
				row.TerminalLink = fmt.Sprintf("/orgs/%s/sources/%s/terminal", org.Name, src.Handle)
			}
			rows = append(rows, row)
		}
	}

	owner, err := db.GetUser(ctx, s.Pool, org.OwnerUserID)
	if err != nil {
		http.Error(w, "owner lookup: "+err.Error(), 500)
		return
	}
	members, err := db.ListMembers(ctx, s.Pool, org.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "org.html", map[string]any{
		"Org":       org,
		"Keys":      keys,
		"Sources":   rows,
		"Owner":     owner,
		"Members":   members,
		"ActiveTab": tab,
	}); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *Server) handleGUICreateKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		http.Error(w, "label required", 400)
		return
	}
	ctx, cancel := withTimeout(r)
	defer cancel()
	org, err := resolveOrg(ctx, s.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, "org not found", 404)
		return
	}
	key, plaintext, err := db.InsertAPIKey(ctx, s.Pool, org.ID, label)
	if err != nil {
		http.Error(w, fmt.Sprintf("create key: %v", err), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "key_created.html", map[string]any{
		"OrgID":     org.ID.String(),
		"OrgName":   org.Name,
		"KeyID":     key.ID.String(),
		"Plaintext": plaintext,
	}); err != nil {
		http.Error(w, err.Error(), 500)
	}
}
