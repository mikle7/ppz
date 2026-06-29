package db

// Auth V2 Phase 2: device-flow + bearer-token storage.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DeviceCode is the row stored in oauth_device_codes.
type DeviceCode struct {
	DeviceCode string
	UserCode   string
	ClientName string
	UserID     *uuid.UUID
	AccountID  *uuid.UUID // org the user picked on the verify page
	ExpiresAt  time.Time
	VerifiedAt *time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// CreateDeviceCode mints a fresh pair, inserts the row, returns it.
// clientName is a free-form label the CLI sends so the verify page
// can name the calling app (e.g. "ppz CLI 0.15.0"); empty string is
// fine — the page falls back to generic copy.
func CreateDeviceCode(ctx context.Context, p *Pool, ttl time.Duration, clientName string) (DeviceCode, error) {
	dc := DeviceCode{
		DeviceCode: generateDeviceCode(),
		UserCode:   generateUserCode(),
		ClientName: clientName,
		ExpiresAt:  time.Now().Add(ttl).UTC(),
		CreatedAt:  time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO oauth_device_codes (device_code, user_code, client_name, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		dc.DeviceCode, dc.UserCode, dc.ClientName, dc.ExpiresAt, dc.CreatedAt)
	return dc, err
}

func LookupDeviceCode(ctx context.Context, p *Pool, deviceCode string) (DeviceCode, error) {
	return scanDeviceCode(p.QueryRow(ctx,
		`SELECT device_code, user_code, client_name, user_id, account_id, expires_at, verified_at, consumed_at, created_at
		   FROM oauth_device_codes WHERE device_code = $1`, deviceCode))
}

func LookupDeviceCodeByUserCode(ctx context.Context, p *Pool, userCode string) (DeviceCode, error) {
	return scanDeviceCode(p.QueryRow(ctx,
		`SELECT device_code, user_code, client_name, user_id, account_id, expires_at, verified_at, consumed_at, created_at
		   FROM oauth_device_codes WHERE user_code = $1`, userCode))
}

func scanDeviceCode(row pgx.Row) (DeviceCode, error) {
	var dc DeviceCode
	err := row.Scan(&dc.DeviceCode, &dc.UserCode, &dc.ClientName, &dc.UserID, &dc.AccountID, &dc.ExpiresAt, &dc.VerifiedAt, &dc.ConsumedAt, &dc.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceCode{}, ErrNotFound
	}
	return dc, err
}

// ApproveDeviceCode marks the user_code as verified by userID, binding it
// to the org accountID the user selected on the verify page. Idempotent.
// Returns ErrNotFound if user_code doesn't exist or is already expired.
func ApproveDeviceCode(ctx context.Context, p *Pool, userCode string, userID, accountID uuid.UUID) error {
	tag, err := p.Exec(ctx,
		`UPDATE oauth_device_codes
		   SET user_id = $2, account_id = $3, verified_at = COALESCE(verified_at, now())
		 WHERE user_code = $1 AND expires_at > now()`,
		userCode, userID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ConsumeDeviceCode is the CLI poll path. Returns the user_id and the
// org the user selected (nil if approved without one) and marks the code
// consumed (single-use). State machine errors:
//   - ErrDeviceCodePending if not yet verified
//   - ErrDeviceCodeExpired if past TTL
//   - ErrNotFound if unknown / already consumed
func ConsumeDeviceCode(ctx context.Context, p *Pool, deviceCode string) (uuid.UUID, *uuid.UUID, error) {
	dc, err := LookupDeviceCode(ctx, p, deviceCode)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if dc.ConsumedAt != nil {
		return uuid.Nil, nil, ErrNotFound
	}
	if time.Now().After(dc.ExpiresAt) {
		return uuid.Nil, nil, ErrDeviceCodeExpired
	}
	if dc.VerifiedAt == nil || dc.UserID == nil {
		return uuid.Nil, nil, ErrDeviceCodePending
	}
	if _, err := p.Exec(ctx,
		`UPDATE oauth_device_codes SET consumed_at = now() WHERE device_code = $1 AND consumed_at IS NULL`,
		deviceCode); err != nil {
		return uuid.Nil, nil, err
	}
	return *dc.UserID, dc.AccountID, nil
}

// ─── bearer tokens ───────────────────────────────────────────────────

type OAuthToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	Prefix     string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func IssueBearerToken(ctx context.Context, p *Pool, userID uuid.UUID, ttl time.Duration) (string, OAuthToken, error) {
	plaintext := generateBearerToken()
	row := OAuthToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashBearerToken(plaintext),
		Prefix:    plaintext[:min(len(plaintext), len("ppz_oauth_")+8)],
		ExpiresAt: time.Now().Add(ttl).UTC(),
		CreatedAt: time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO oauth_tokens (id, user_id, token_hash, token_prefix, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		row.ID, row.UserID, row.TokenHash, row.Prefix, row.ExpiresAt, row.CreatedAt)
	return plaintext, row, err
}

func LookupBearerToken(ctx context.Context, p *Pool, plaintext string) (OAuthToken, error) {
	hash := hashBearerToken(plaintext)
	var t OAuthToken
	err := p.QueryRow(ctx,
		`SELECT id, user_id, token_hash, token_prefix, expires_at, revoked_at, created_at, last_used_at
		   FROM oauth_tokens
		  WHERE token_hash = $1`, hash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.Prefix, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthToken{}, ErrNotFound
	}
	if err != nil {
		return OAuthToken{}, err
	}
	if t.RevokedAt != nil {
		return OAuthToken{}, ErrNotFound
	}
	if time.Now().After(t.ExpiresAt) {
		return OAuthToken{}, ErrNotFound
	}
	// Best-effort last_used_at update (don't block on its failure).
	_, _ = p.Exec(ctx, `UPDATE oauth_tokens SET last_used_at = now() WHERE id = $1`, t.ID)
	return t, nil
}

func hashBearerToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ─── sentinel errors ─────────────────────────────────────────────────

var (
	ErrDeviceCodePending = errors.New("device_code not yet verified")
	ErrDeviceCodeExpired = errors.New("device_code expired")
)

// ─── generators ──────────────────────────────────────────────────────

const userCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I/L

// generateUserCode produces XXXX-XXXX from the unambiguous alphabet.
func generateUserCode() string {
	buf := make([]byte, 9)
	rb := make([]byte, 8)
	if _, err := rand.Read(rb); err != nil {
		panic(err) // /dev/urandom unavailable — bigger problem than we can recover from
	}
	for i, b := range rb {
		idx := i
		if i >= 4 {
			idx = i + 1
		}
		buf[idx] = userCodeAlphabet[int(b)%len(userCodeAlphabet)]
	}
	buf[4] = '-'
	return string(buf)
}

// generateDeviceCode produces 32 url-safe random bytes (≈43 chars
// after base64-url-no-pad).
func generateDeviceCode() string {
	rb := make([]byte, 32)
	if _, err := rand.Read(rb); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(rb)
}

// generateBearerToken: ppz_oauth_<48 url-safe random chars>.
func generateBearerToken() string {
	rb := make([]byte, 36) // 36*8/6 ≈ 48 chars after base64-no-pad
	if _, err := rand.Read(rb); err != nil {
		panic(err)
	}
	return "ppz_oauth_" + base64.RawURLEncoding.EncodeToString(rb)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
