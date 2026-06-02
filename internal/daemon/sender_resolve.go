package daemon

// senderForRequest picks envelope.sender for a `ppz send` / `ppz command`
// publish. Rule: an explicit hint from the CLI wins; otherwise fall
// back to the daemon's per-session state.
//
// Why a hint at all: PPZ_CURRENT_HANDLE is the source of truth for "who
// am I right now?" inside a `ppz terminal share` wrapped shell —
// terminalShareEnv exports both PPZ_CURRENT_HANDLE=H and PPZ_SESSION=H,
// and `ppz status` / `ppz read inbox` / the `--request-ack` preflight
// all read env-first via effectiveCurrentHandle. But IPCCreate for a
// PTY-kind source deliberately skips SetCurrent, so the daemon's own
// State.Current(session) is empty even though every other verb agrees
// the wrapped handle is current. `ppz send` (alone among the verbs)
// used to resolve sender via daemon-state only, stamping
// envelope.sender="" on every publish from inside a share. The CLI
// now forwards its env-resolved current as SendRequest.Sender; this
// helper centralises the precedence so future stamp sites can't
// accidentally resurrect the asymmetry.
func senderForRequest(reqSender, stateCurrent string) string {
	if reqSender != "" {
		return reqSender
	}
	return stateCurrent
}
