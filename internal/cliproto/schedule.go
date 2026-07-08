package cliproto

// Scheduled sends (docs/specs/schedule.md): IPC types + printers for
// `ppz send --at/--every/--cron` and `ppz schedule {ls|rm}`. The table
// follows the `ppz ls` conventions — see PrintList — with a leading ID
// column (the `schedule rm` handle) and NEXT/LAST time columns.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// IPC verbs. Wire strings — an old CLI against a new daemon matches on
// these literals.
const (
	IPCScheduleCreate = "ScheduleCreate"
	IPCScheduleList   = "ScheduleList"
	IPCScheduleRemove = "ScheduleRemove"
)

// ScheduleCreateRequest mirrors SendRequest's target fields (the
// daemon runs the same resolution pre-flight) plus the schedule
// shape. Kind is "at" | "every" | "cron" with exactly one of
// At/Every/Cron set to match:
//
//   - At: RFC3339, creator's offset preserved (the CLI resolves
//     relative/local forms before IPC)
//   - Every: Go duration string, validated ≥1s client-side
//   - Cron: standard 5-field expression; TZ carries the device's
//     IANA zone name
type ScheduleCreateRequest struct {
	Session    string `json:"session,omitempty"`
	Sender     string `json:"sender,omitempty"` // PPZ_CURRENT_HANDLE hint, as on SendRequest
	Handle     string `json:"handle"`
	Channel    string `json:"channel"`
	BareTarget string `json:"bare_target,omitempty"`
	Payload    string `json:"payload"`
	Kind       string `json:"kind"`
	At         string `json:"at,omitempty"`
	Every      string `json:"every,omitempty"`
	Cron       string `json:"cron,omitempty"`
	TZ         string `json:"tz,omitempty"`
}

// ScheduleCreateReply carries what the success line prints: the short
// id (the `schedule rm` handle), the resolved display target, and the
// first fire instant.
type ScheduleCreateReply struct {
	ID     string    `json:"id"`
	Target string    `json:"target"`
	NextAt time.Time `json:"next_at"`
}

// ScheduleInfo is one `schedule ls` row. Namespace/Handle/Pipe carry
// the ls-table semantics: Namespace is the raw manifold ("" for root —
// renderers dash it), Handle is "" for uncollared targets, Pipe the
// leaf. Spec is the display spec (RFC3339 for at, duration for every,
// expression for cron); TZ is set for cron only. LastAt is nil when
// the schedule has never fired. Payload is full-fidelity — the table
// renderer truncates, the JSON printer doesn't.
type ScheduleInfo struct {
	ID        string     `json:"id"`
	Namespace string     `json:"namespace"`
	Handle    string     `json:"handle"`
	Pipe      string     `json:"pipe"`
	Kind      string     `json:"schedule"`
	Spec      string     `json:"spec"`
	TZ        string     `json:"tz"`
	NextAt    time.Time  `json:"next_at"`
	LastAt    *time.Time `json:"last_at"`
	Payload   string     `json:"payload"`
	Creator   string     `json:"creator"`
}

// ScheduleServerCreateRequest is the daemon → server REST body
// (POST /api/v1/schedules). The daemon has already resolved the
// target: Handle=="" means an uncollared pipe at Manifold.
type ScheduleServerCreateRequest struct {
	Manifold string `json:"manifold"`
	Handle   string `json:"handle"`
	Pipe     string `json:"pipe"`
	Payload  string `json:"payload"`
	Sender   string `json:"sender"`
	Kind     string `json:"kind"`
	At       string `json:"at,omitempty"`
	Every    string `json:"every,omitempty"`
	Cron     string `json:"cron,omitempty"`
	TZ       string `json:"tz,omitempty"`
}

type ScheduleListRequest struct {
	Session string `json:"session,omitempty"`
}

type ScheduleListReply struct {
	Schedules []ScheduleInfo `json:"schedules"`
}

type ScheduleRemoveRequest struct {
	Session string `json:"session,omitempty"`
	ID      string `json:"id"`
}

type ScheduleRemoveReply struct {
	ID string `json:"id"`
}

// RelativeFuture is the forward-looking sibling of RelativeTime, used
// by the NEXT column. Same unit boundaries and pluralisation; sub-
// second reads "now".
func RelativeFuture(t, now time.Time) string {
	d := t.Sub(now)
	if d < time.Second {
		return "now"
	}
	if d < time.Minute {
		n := int(d / time.Second)
		return fmt.Sprintf("in %d %s", n, pluralUnit(n, "second"))
	}
	if d < time.Hour {
		n := int(d / time.Minute)
		return fmt.Sprintf("in %d %s", n, pluralUnit(n, "minute"))
	}
	if d < 24*time.Hour {
		n := int(d / time.Hour)
		return fmt.Sprintf("in %d %s", n, pluralUnit(n, "hour"))
	}
	n := int(d / (24 * time.Hour))
	return fmt.Sprintf("in %d %s", n, pluralUnit(n, "day"))
}

// PrintScheduleCreate prints the pinned creation line:
//
//	scheduled id=<id8> to=<path> next=<RFC3339 UTC>
func PrintScheduleCreate(w io.Writer, r ScheduleCreateReply) {
	fmt.Fprintf(w, "scheduled id=%s to=%s next=%s\n", r.ID, r.Target, r.NextAt.UTC().Format(time.RFC3339))
}

// PrintScheduleRemove prints the pinned removal line:
//
//	removed schedule=<id8>
func PrintScheduleRemove(w io.Writer, r ScheduleRemoveReply) {
	fmt.Fprintf(w, "removed schedule=%s\n", r.ID)
}

// scheduleCell renders the SCHEDULE column: `at <spec>`, `every <dur>`,
// or `cron <expr> <tz>`.
func scheduleCell(s ScheduleInfo) string {
	cell := s.Kind + " " + s.Spec
	if s.Kind == "cron" && s.TZ != "" {
		cell += " " + s.TZ
	}
	return cell
}

// sortedSchedules returns a copy ordered by NextAt ascending (soonest
// first — the question the table answers), id as the deterministic
// tie-break. Both printers share it so table and JSON agree.
func sortedSchedules(schedules []ScheduleInfo) []ScheduleInfo {
	out := append([]ScheduleInfo(nil), schedules...)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].NextAt.Equal(out[j].NextAt) {
			return out[i].NextAt.Before(out[j].NextAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// PrintScheduleList renders `schedule ls` as an aligned table with the
// `ppz ls` conventions: header only when rows exist, two-space gaps,
// display-width padding, "-" for missing values, relative NEXT/LAST by
// default (iso=true flips both to RFC3339 UTC), trailing CREATOR
// column unpadded, payload preview truncated.
func PrintScheduleList(w io.Writer, schedules []ScheduleInfo, iso bool) {
	if len(schedules) == 0 {
		return
	}
	now := timeNow()
	headers := []string{"ID", "NAMESPACE", "PIPE", "SCHEDULE", "NEXT", "LAST", "PAYLOAD", "CREATOR"}
	type row struct{ cells [8]string }
	rows := make([]row, 0, len(schedules))
	for _, s := range sortedSchedules(schedules) {
		next := RelativeFuture(s.NextAt, now)
		if iso {
			next = s.NextAt.UTC().Format(time.RFC3339)
		}
		rows = append(rows, row{cells: [8]string{
			s.ID,
			namespaceColumn(s.Namespace),
			FormatPipePath("", s.Handle, s.Pipe),
			scheduleCell(s),
			next,
			lastColumn(s.LastAt, now, iso),
			payloadColumn(TruncatePayload(s.Payload)),
			s.Creator,
		}})
	}
	widths := make([]int, 7) // CREATOR trails bare — no width tracking
	for i := 0; i < 7; i++ {
		widths[i] = dispWidth(headers[i])
	}
	for _, r := range rows {
		for i := 0; i < 7; i++ {
			if l := dispWidth(r.cells[i]); l > widths[i] {
				widths[i] = l
			}
		}
	}
	line := func(cells [8]string) {
		for i := 0; i < 7; i++ {
			fmt.Fprintf(w, "%s  ", padRightDisp(cells[i], widths[i]))
		}
		fmt.Fprintf(w, "%s\n", cells[7])
	}
	line([8]string{headers[0], headers[1], headers[2], headers[3], headers[4], headers[5], headers[6], headers[7]})
	for _, r := range rows {
		line(r.cells)
	}
}

// PrintScheduleListJSON is the `schedule ls --json` variant: one JSONL
// object per schedule, same order as the table, full untruncated
// payload, RFC3339-UTC next_at, null last_at when never fired.
func PrintScheduleListJSON(w io.Writer, schedules []ScheduleInfo) {
	for _, s := range sortedSchedules(schedules) {
		obj := map[string]any{
			"id":        s.ID,
			"namespace": s.Namespace,
			"handle":    s.Handle,
			"pipe":      s.Pipe,
			"schedule":  s.Kind,
			"spec":      s.Spec,
			"tz":        s.TZ,
			"next_at":   s.NextAt.UTC().Format(time.RFC3339),
			"payload":   s.Payload,
			"creator":   s.Creator,
		}
		if s.LastAt != nil {
			obj["last_at"] = s.LastAt.UTC().Format(time.RFC3339)
		} else {
			obj["last_at"] = nil
		}
		line, _ := json.Marshal(obj)
		fmt.Fprintln(w, string(line))
	}
}
