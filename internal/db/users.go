package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UserMode = "github" (real OAuth identity) or "internal" (placeholder /
// seeded test user / pre-OAuth-era account).
type UserMode string

const (
	UserModeGithub   UserMode = "github"
	UserModeInternal UserMode = "internal"
)

type User struct {
	ID        uuid.UUID
	Username  string
	Email     string
	Mode      UserMode
	GitHubID  *int64 // nil for mode=internal users
	AvatarURL string
	CreatedAt time.Time
}

// ErrInvalidUserMode is returned when a caller passes a Mode value
// outside the {github, internal} CHECK constraint.
var ErrInvalidUserMode = errors.New("user mode must be 'github' or 'internal'")

func InsertUser(ctx context.Context, p *Pool, username, email string, mode UserMode) (User, error) {
	if mode != UserModeGithub && mode != UserModeInternal {
		return User{}, ErrInvalidUserMode
	}
	u := User{
		ID:        uuid.New(),
		Username:  username,
		Email:     email,
		Mode:      mode,
		CreatedAt: time.Now().UTC(),
	}
	_, err := p.Exec(ctx,
		`INSERT INTO users (id, username, email, mode, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Username, u.Email, string(u.Mode), u.CreatedAt)
	return u, err
}

func GetUser(ctx context.Context, p *Pool, id uuid.UUID) (User, error) {
	var u User
	var mode string
	err := p.QueryRow(ctx,
		`SELECT id, username, email, mode, github_id, COALESCE(avatar_url,''), created_at
		   FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Username, &u.Email, &mode, &u.GitHubID, &u.AvatarURL, &u.CreatedAt)
	u.Mode = UserMode(mode)
	return u, err
}

func GetUserByUsername(ctx context.Context, p *Pool, username string) (User, error) {
	var u User
	var mode string
	err := p.QueryRow(ctx,
		`SELECT id, username, email, mode, created_at FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.Email, &mode, &u.CreatedAt)
	u.Mode = UserMode(mode)
	return u, err
}

// UpsertUserByGitHubID inserts a brand new mode=github user, or
// updates the existing row matching the GitHub numeric id. Returns
// the resolved User row plus a bool indicating whether the row was
// freshly created (true) vs already existed (false). Callers use
// the bool to decide whether to auto-create the user's first org.
func UpsertUserByGitHubID(ctx context.Context, p *Pool, githubID int64, username, email, avatarURL string) (User, bool, error) {
	// Existing row?
	var u User
	var mode string
	err := p.QueryRow(ctx,
		`SELECT id, username, email, mode, github_id, avatar_url, created_at
		   FROM users WHERE github_id = $1`, githubID).
		Scan(&u.ID, &u.Username, &u.Email, &mode, &u.GitHubID, &u.AvatarURL, &u.CreatedAt)

	if err == nil {
		u.Mode = UserMode(mode)
		// Refresh email + avatar in case the user changed them on GitHub.
		if _, err := p.Exec(ctx,
			`UPDATE users SET email = $2, avatar_url = $3 WHERE id = $1`,
			u.ID, email, avatarURL); err != nil {
			return User{}, false, err
		}
		u.Email = email
		u.AvatarURL = avatarURL
		return u, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, err
	}

	// Fresh row.
	id := uuid.New()
	now := time.Now().UTC()
	gh := githubID
	u = User{
		ID:        id,
		Username:  username,
		Email:     email,
		Mode:      UserModeGithub,
		GitHubID:  &gh,
		AvatarURL: avatarURL,
		CreatedAt: now,
	}
	if _, err := p.Exec(ctx,
		`INSERT INTO users (id, username, email, mode, github_id, avatar_url, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Username, u.Email, string(u.Mode), gh, avatarURL, now); err != nil {
		return User{}, false, err
	}
	return u, true, nil
}