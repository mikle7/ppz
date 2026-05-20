package daemon

// resolveCallerSession is the small wrapper most handlers use: given a
// (declaredSession, ancestorPIDs) tuple from an IPC request, return
// the effective session key for State lookups.
//
// Backwards compatibility:
//   - Old CLI sends Session non-empty, AncestorPIDs nil → returns Session
//     verbatim. Legacy passthrough.
//   - New CLI sends Session empty, AncestorPIDs populated → engages the
//     resolver + auto-write current handle.
//   - Old daemons receiving new-CLI requests ignore AncestorPIDs; new
//     CLI must still populate Session for old-daemon back-compat.
//
// Returns just the session key — handlers that also need the
// resolved sender call resolveSenderForSend(state, key).
func (d *Daemon) resolveCallerSession(declaredSession string, ancestorPIDs []int) string {
	r := d.State.ResolveSessionWithAutoWrite(declaredSession, ancestorPIDs)
	return r.SessionKey
}
