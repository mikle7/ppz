// Package daemon implements the long-lived ppz daemon: IPC server, on-disk
// state, NATS connection, and HTTP client to ppz-server.
package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// Credentials are persisted at $PPZ_HOME/credentials (mode 0600). AccountID +
// AccountName are stored alongside URL+APIKey so a SIGHUP / file-poller reload
// doesn't drop the resolved org info (the alternative would be re-calling
// /auth/exchange after every reload).
type Credentials struct {
	URL     string `json:"url"`
	APIKey  string `json:"api_key"`
	AccountID   string `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`

	// Auth V2 Phase 3 — short-lived NATS user credentials.
	// Re-fetched periodically by the daemon's refresh goroutine.
	NATSUserJWT  string `json:"nats_user_jwt,omitempty"`
	NATSUserSeed string `json:"nats_user_seed,omitempty"`
}

// Files under $PPZ_HOME the daemon owns. Names are part of the WIRE.md §9
// contract — the test reset script deletes these by name.
const (
	fileCredentials = "credentials"
	fileCurrent     = "current.json" // session → handle map (was "current")
	filePID         = "daemon.pid"
	fileSocket      = "daemon.sock"
)

// State is the daemon's in-memory mirror of on-disk credentials + current
// handle. "current" is keyed by session id (tty / $PPZ_SESSION) so each
// terminal window has its own current source — same scoping as cursors,
// avoids the "new terminal silently inherits a stale current set hours
// ago in another window" footgun.
type State struct {
	mu          sync.RWMutex
	home        string
	creds       *Credentials
	accountID       string // resolved at Login (returned by /auth/exchange)
	accountName     string // resolved at Login (human label for status)
	keyPrefix   string // 8 chars; safe to display
	current     map[string]string
	knownPipes  map[string]struct{} // server-side handles cached after List/Create
	pipesLoaded bool
	// loginCheck caches the daemon's last server-touching call result.
	// Empty means "no observation yet" (fresh daemon). LoginCheckOK on
	// successful 2xx; LoginCheckInvalid on 401 / E_INVALID_API_KEY.
	// Surfaced by `ppz status` so an already-known auth failure shows
	// up immediately, without status needing its own probe.
	loginCheck string
}

// normSession matches cursors.session — empty session id means "default".
func normSession(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

func NewState(home string) *State {
	return &State{home: home, current: map[string]string{}, knownPipes: map[string]struct{}{}}
}

func (s *State) Home() string { return s.home }

func (s *State) Credentials() (*Credentials, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.creds == nil {
		return nil, false
	}
	c := *s.creds
	return &c, true
}

func (s *State) AccountID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountID
}

func (s *State) AccountName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountName
}

// LoginCheck returns the cached server-validation result. "" means "not
// observed yet" — callers (e.g. status) should probe.
func (s *State) LoginCheck() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loginCheck
}

// SetLoginCheck records the latest server-validation result. Called from
// callServer based on response status. Idempotent — same value twice is
// fine, but transitions (ok→invalid, invalid→ok) are kept.
func (s *State) SetLoginCheck(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loginCheck = state
}

func (s *State) KeyPrefix() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyPrefix
}

func (s *State) Current(session string) string {
	session = normSession(session)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current[session]
}

func (s *State) SetLogin(creds Credentials, accountID, accountName, keyPrefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	creds.AccountID = accountID
	creds.AccountName = accountName
	s.creds = &creds
	s.accountID = accountID
	s.accountName = accountName
	s.keyPrefix = keyPrefix
	s.knownPipes = map[string]struct{}{}
	s.pipesLoaded = false
	// Login itself is a successful server round-trip — record it as the
	// initial "ok" observation so status shows the right state right away.
	s.loginCheck = cliproto.LoginCheckOK
	return s.persistCredsLocked()
}

func (s *State) SetCurrent(session, handle string) error {
	session = normSession(session)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current[session] = handle
	return s.persistCurrentLocked()
}

// ClearCurrent drops this session's current. Used by `ppz disconnect`.
// Idempotent — clearing an already-clear session is a no-op.
func (s *State) ClearCurrent(session string) error {
	session = normSession(session)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.current[session]; !ok {
		return nil
	}
	delete(s.current, session)
	return s.persistCurrentLocked()
}

func (s *State) RememberPipe(handle string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knownPipes[handle] = struct{}{}
}

// ForgetPipe removes a handle from the known-pipes cache. Called after a
// source is destroyed so the cache doesn't return stale hits.
func (s *State) ForgetPipe(handle string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.knownPipes, handle)
}

// ClearCurrentForHandle drops every session whose current equals handle.
// Called after a source destroy so stale per-session pointers don't linger.
func (s *State) ClearCurrentForHandle(handle string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for sess, h := range s.current {
		if h == handle {
			delete(s.current, sess)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.persistCurrentLocked()
}

func (s *State) KnowsPipe(handle string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.knownPipes[handle]
	return ok
}

func (s *State) ResetPipes(handles []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knownPipes = make(map[string]struct{}, len(handles))
	for _, h := range handles {
		s.knownPipes[h] = struct{}{}
	}
	s.pipesLoaded = true
}

// LoadFromDisk reads credentials and current from $PPZ_HOME. Called at
// startup and on SIGHUP. Missing files mean "not logged in" / "no current".
//
// `current.json` is the session→handle map. The legacy plain-text `current`
// file (pre-per-session) is migrated into session "default" if both exist.
func (s *State) LoadFromDisk() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds = nil
	s.accountID = ""
	s.accountName = ""
	s.keyPrefix = ""
	s.current = map[string]string{}
	s.knownPipes = map[string]struct{}{}
	s.pipesLoaded = false
	// Reload zeros the cache: a daemon that just woke up hasn't talked to
	// the server yet under the new credentials, so status should probe.
	s.loginCheck = ""

	if data, err := os.ReadFile(filepath.Join(s.home, fileCredentials)); err == nil {
		var c Credentials
		if err := json.Unmarshal(data, &c); err == nil {
			s.creds = &c
			s.keyPrefix = keyPrefix(c.APIKey)
			s.accountID = c.AccountID
			s.accountName = c.AccountName
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if data, err := os.ReadFile(filepath.Join(s.home, fileCurrent)); err == nil {
		_ = json.Unmarshal(data, &s.current)
		if s.current == nil {
			s.current = map[string]string{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Legacy migration: pre-per-session daemons stored a single plain-text
	// "current" file. If we find it, fold it into the "default" session.
	if data, err := os.ReadFile(filepath.Join(s.home, "current")); err == nil {
		if h := strings.TrimSpace(string(data)); h != "" {
			if _, ok := s.current["default"]; !ok {
				s.current["default"] = h
			}
		}
	}
	return nil
}

func (s *State) SetAccountID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountID = id
}

func (s *State) persistCurrentLocked() error {
	// Atomic-ish: write tmp then rename. Mirrors cursors.saveLocked().
	data, err := json.Marshal(s.current)
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.home, fileCurrent+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.home, fileCurrent))
}

func (s *State) persistCredsLocked() error {
	if s.creds == nil {
		_ = os.Remove(filepath.Join(s.home, fileCredentials))
		return nil
	}
	data, err := json.Marshal(s.creds)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.home, fileCredentials), data, 0o600)
}

func keyPrefix(plaintext string) string {
	s := strings.TrimPrefix(plaintext, "ppz_")
	if len(s) < 8 {
		return s
	}
	return s[:8]
}
