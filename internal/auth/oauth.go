package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sign-in for AI assistants over MCP (OAuth 2.1 with PKCE). The HTTP side lives
// in internal/api/mcp_oauth.go; this file keeps the clients, the grants (one
// administrator saying yes to one client) and the tokens, which are random
// values stored only as a hash.

const (
	// OAuthAccessTTL is how long an access token works.
	OAuthAccessTTL = time.Hour
	// OAuthRefreshTTL is how long a refresh token works.
	OAuthRefreshTTL = 90 * 24 * time.Hour
	// MaxOAuthClients is the most clients that can be registered.
	MaxOAuthClients = 200

	oauthAccessPrefix  = "mcpat_"
	oauthRefreshPrefix = "mcprt_"
)

var (
	// ErrOAuthInvalidGrant is a refresh token that is unknown, expired, used
	// before, or belongs to a revoked grant or another client.
	ErrOAuthInvalidGrant = errors.New("the token is not valid")
	// ErrOAuthTooManyClients is returned when no more clients can be registered.
	ErrOAuthTooManyClients = errors.New("too many registered clients")
)

// OAuthClient is an app that may ask for a sign-in.
type OAuthClient struct {
	ID            string
	Name          string
	RedirectURIs  []string
	Confidential  bool // has a secret
	Preregistered bool
	CreatedAt     time.Time
}

// OAuthGrant is one administrator's yes to one client.
type OAuthGrant struct {
	ID         int64
	ClientID   string
	ClientName string
	UserID     int64
	Username   string
	Scope      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// OAuthTokens is a new pair, shown once.
type OAuthTokens struct {
	Access    string
	Refresh   string
	AccessTTL time.Duration
	Scope     string
	GrantID   int64
}

func oauthRandom(prefix string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// NewOAuthClientID makes a client id.
func NewOAuthClientID() (string, error) { return oauthRandom("mcpc_", 18) }

// NewOAuthSecret makes a client secret.
func NewOAuthSecret() (string, error) { return oauthRandom("mcps_", 32) }

// HasOAuthTokenPrefix reports whether a bearer value looks like one of our OAuth access tokens.
func HasOAuthTokenPrefix(v string) bool { return strings.HasPrefix(v, oauthAccessPrefix) }

// CreateOAuthClient stores a client. A secret of "" makes it a public client.
func (s *Service) CreateOAuthClient(id, secret, name string, redirectURIs []string, preregistered bool) error {
	raw, err := json.Marshal(redirectURIs)
	if err != nil {
		return fmt.Errorf("encode redirect addresses: %w", err)
	}
	hash := ""
	if secret != "" {
		hash = hashToken(secret)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM oauth_clients`).Scan(&n); err != nil {
		return fmt.Errorf("count clients: %w", err)
	}
	if n >= MaxOAuthClients {
		// Make room: clients that registered themselves and were never approved by anybody.
		cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
		if _, err := s.db.Exec(`DELETE FROM oauth_clients WHERE preregistered = 0 AND created_at < ?
			AND client_id NOT IN (SELECT client_id FROM oauth_grants)`, cutoff); err != nil {
			return fmt.Errorf("clean up clients: %w", err)
		}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM oauth_clients`).Scan(&n); err != nil {
			return fmt.Errorf("count clients: %w", err)
		}
		if n >= MaxOAuthClients {
			return ErrOAuthTooManyClients
		}
	}
	pre := 0
	if preregistered {
		pre = 1
	}
	_, err = s.db.Exec(`INSERT INTO oauth_clients (client_id, secret_hash, name, redirect_uris, preregistered, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, hash, name, string(raw), pre, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save client: %w", err)
	}
	return nil
}

func scanOAuthClient(sc interface{ Scan(...any) error }) (*OAuthClient, string, error) {
	var (
		c       OAuthClient
		secret  string
		uris    string
		pre     bool
		created string
	)
	if err := sc.Scan(&c.ID, &secret, &c.Name, &uris, &pre, &created); err != nil {
		return nil, "", err
	}
	_ = json.Unmarshal([]byte(uris), &c.RedirectURIs)
	c.Confidential = secret != ""
	c.Preregistered = pre
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return &c, secret, nil
}

// OAuthClientByID returns a client and whether it exists.
func (s *Service) OAuthClientByID(id string) (*OAuthClient, bool, error) {
	c, _, err := scanOAuthClient(s.db.QueryRow(`SELECT client_id, secret_hash, name, redirect_uris, preregistered, created_at FROM oauth_clients WHERE client_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("look up client: %w", err)
	}
	return c, true, nil
}

// CheckOAuthClientSecret reports whether secret is the client's. A public
// client (no secret) accepts only an empty one.
func (s *Service) CheckOAuthClientSecret(id, secret string) bool {
	var stored string
	if err := s.db.QueryRow(`SELECT secret_hash FROM oauth_clients WHERE client_id = ?`, id).Scan(&stored); err != nil {
		return false
	}
	if stored == "" {
		return secret == ""
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(hashToken(secret))) == 1
}

// ListOAuthClients lists the clients an administrator made by hand.
func (s *Service) ListOAuthClients() ([]OAuthClient, error) {
	rows, err := s.db.Query(`SELECT client_id, secret_hash, name, redirect_uris, preregistered, created_at FROM oauth_clients WHERE preregistered = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()
	var out []OAuthClient
	for rows.Next() {
		c, _, err := scanOAuthClient(rows)
		if err != nil {
			return nil, fmt.Errorf("read client: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// DeleteOAuthClient removes a client with all its grants and tokens.
func (s *Service) DeleteOAuthClient(id string) (bool, error) {
	if _, err := s.db.Exec(`DELETE FROM oauth_tokens WHERE grant_id IN (SELECT id FROM oauth_grants WHERE client_id = ?)`, id); err != nil {
		return false, fmt.Errorf("delete tokens: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM oauth_grants WHERE client_id = ?`, id); err != nil {
		return false, fmt.Errorf("delete grants: %w", err)
	}
	res, err := s.db.Exec(`DELETE FROM oauth_clients WHERE client_id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete client: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CreateOAuthGrant records an administrator's approval and returns the first token pair.
func (s *Service) CreateOAuthGrant(clientID string, userID int64, scope string) (*OAuthTokens, error) {
	res, err := s.db.Exec(`INSERT INTO oauth_grants (client_id, user_id, scope, created_at) VALUES (?, ?, ?, ?)`,
		clientID, userID, scope, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("save grant: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.issueOAuthTokens(id, scope)
}

func (s *Service) issueOAuthTokens(grantID int64, scope string) (*OAuthTokens, error) {
	access, err := oauthRandom(oauthAccessPrefix, 32)
	if err != nil {
		return nil, err
	}
	refresh, err := oauthRandom(oauthRefreshPrefix, 32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tokens: %w", err)
	}
	defer tx.Rollback()
	// Old access tokens and spent refresh tokens of this grant are no longer needed
	// (a spent refresh token is kept to spot a reuse, see RefreshOAuth).
	if _, err := tx.Exec(`DELETE FROM oauth_tokens WHERE expires_at < ?`, now.Format(time.RFC3339Nano)); err != nil {
		return nil, fmt.Errorf("clean up tokens: %w", err)
	}
	for _, t := range []struct {
		v, kind string
		ttl     time.Duration
	}{
		{access, "access", OAuthAccessTTL},
		{refresh, "refresh", OAuthRefreshTTL},
	} {
		if _, err := tx.Exec(`INSERT INTO oauth_tokens (hash, grant_id, kind, expires_at) VALUES (?, ?, ?, ?)`,
			hashToken(t.v), grantID, t.kind, now.Add(t.ttl).Format(time.RFC3339Nano)); err != nil {
			return nil, fmt.Errorf("save token: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tokens: %w", err)
	}
	return &OAuthTokens{Access: access, Refresh: refresh, AccessTTL: OAuthAccessTTL, Scope: scope, GrantID: grantID}, nil
}

// RefreshOAuth swaps a refresh token for a new pair. A refresh token works once;
// presenting one that was already used means it leaked, so the whole grant is
// revoked.
func (s *Service) RefreshOAuth(clientID, refresh string) (*OAuthTokens, error) {
	if !strings.HasPrefix(refresh, oauthRefreshPrefix) {
		return nil, ErrOAuthInvalidGrant
	}
	var (
		grantID                  int64
		expires, used, scope, cl string
		revoked                  sql.NullString
		userID                   int64
	)
	err := s.db.QueryRow(`SELECT t.grant_id, t.expires_at, t.used_at, g.scope, g.client_id, g.revoked_at, g.user_id
		FROM oauth_tokens t JOIN oauth_grants g ON g.id = t.grant_id
		WHERE t.hash = ? AND t.kind = 'refresh'`, hashToken(refresh)).Scan(&grantID, &expires, &used, &scope, &cl, &revoked, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthInvalidGrant
	}
	if err != nil {
		return nil, fmt.Errorf("look up refresh token: %w", err)
	}
	if cl != clientID || revoked.Valid {
		return nil, ErrOAuthInvalidGrant
	}
	if used != "" {
		if err := s.RevokeOAuthGrant(grantID); err != nil {
			return nil, err
		}
		return nil, ErrOAuthInvalidGrant
	}
	if exp, _ := time.Parse(time.RFC3339Nano, expires); time.Now().After(exp) {
		return nil, ErrOAuthInvalidGrant
	}
	if _, err := s.oauthUser(userID); err != nil {
		return nil, ErrOAuthInvalidGrant
	}
	// Mark it used in the same step that checks it, so two requests can't both win.
	res, err := s.db.Exec(`UPDATE oauth_tokens SET used_at = ? WHERE hash = ? AND used_at = ''`, time.Now().UTC().Format(time.RFC3339Nano), hashToken(refresh))
	if err != nil {
		return nil, fmt.Errorf("use refresh token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_ = s.RevokeOAuthGrant(grantID)
		return nil, ErrOAuthInvalidGrant
	}
	// The access tokens of the old pair stop here; the new pair replaces them.
	if _, err := s.db.Exec(`DELETE FROM oauth_tokens WHERE grant_id = ? AND kind = 'access'`, grantID); err != nil {
		return nil, fmt.Errorf("replace access tokens: %w", err)
	}
	return s.issueOAuthTokens(grantID, scope)
}

// oauthUser loads an account that may still use a grant: an enabled administrator.
func (s *Service) oauthUser(id int64) (*User, error) {
	u, err := s.GetUser(id)
	if err != nil {
		return nil, err
	}
	if u.Disabled || !u.IsAdmin {
		return nil, ErrSessionNotFound
	}
	return u, nil
}

// UserForOAuthToken resolves an access token to its administrator. A token whose
// scope has no mcp:write is read-only, like a read-only API key.
func (s *Service) UserForOAuthToken(token string) (*User, error) {
	if !strings.HasPrefix(token, oauthAccessPrefix) {
		return nil, ErrSessionNotFound
	}
	var (
		grantID, userID int64
		expires, scope  string
		lastUsed        string
	)
	err := s.db.QueryRow(`SELECT t.grant_id, t.expires_at, g.scope, g.user_id, g.last_used_at
		FROM oauth_tokens t JOIN oauth_grants g ON g.id = t.grant_id
		WHERE t.hash = ? AND t.kind = 'access' AND g.revoked_at IS NULL`, hashToken(token)).Scan(&grantID, &expires, &scope, &userID, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("look up access token: %w", err)
	}
	if exp, _ := time.Parse(time.RFC3339Nano, expires); time.Now().After(exp) {
		return nil, ErrSessionNotFound
	}
	u, err := s.oauthUser(userID)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	u.ReadOnlyKey = !hasScope(scope, "mcp:write")
	if last, _ := time.Parse(time.RFC3339Nano, lastUsed); time.Since(last) > time.Minute {
		_, _ = s.db.Exec(`UPDATE oauth_grants SET last_used_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), grantID)
	}
	return u, nil
}

func hasScope(scope, want string) bool {
	for _, f := range strings.Fields(scope) {
		if f == want {
			return true
		}
	}
	return false
}

// ListOAuthGrants lists the connected apps that are not revoked.
func (s *Service) ListOAuthGrants() ([]OAuthGrant, error) {
	rows, err := s.db.Query(`SELECT g.id, g.client_id, COALESCE(c.name, ''), g.user_id, COALESCE(u.username, ''), g.scope, g.created_at, g.last_used_at
		FROM oauth_grants g LEFT JOIN oauth_clients c ON c.client_id = g.client_id LEFT JOIN users u ON u.id = g.user_id
		WHERE g.revoked_at IS NULL ORDER BY g.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	defer rows.Close()
	var out []OAuthGrant
	for rows.Next() {
		var (
			g             OAuthGrant
			created, used string
		)
		if err := rows.Scan(&g.ID, &g.ClientID, &g.ClientName, &g.UserID, &g.Username, &g.Scope, &created, &used); err != nil {
			return nil, fmt.Errorf("read grant: %w", err)
		}
		g.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if t, err := time.Parse(time.RFC3339Nano, used); err == nil {
			g.LastUsedAt = &t
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// RevokeOAuthGrant ends a grant: its tokens stop working at once.
func (s *Service) RevokeOAuthGrant(id int64) error {
	if _, err := s.db.Exec(`UPDATE oauth_grants SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("revoke grant: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM oauth_tokens WHERE grant_id = ?`, id); err != nil {
		return fmt.Errorf("remove tokens: %w", err)
	}
	return nil
}

// RevokeOAuthGrantByID is RevokeOAuthGrant that reports whether a live grant was there.
func (s *Service) RevokeOAuthGrantByID(id int64) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM oauth_grants WHERE id = ? AND revoked_at IS NULL`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up grant: %w", err)
	}
	return true, s.RevokeOAuthGrant(id)
}

// revokeUserGrants ends every grant of an account (password change, removal).
func revokeUserGrants(exec func(string, ...any) (sql.Result, error), userID int64) error {
	if _, err := exec(`DELETE FROM oauth_tokens WHERE grant_id IN (SELECT id FROM oauth_grants WHERE user_id = ?)`, userID); err != nil {
		return fmt.Errorf("remove tokens: %w", err)
	}
	if _, err := exec(`DELETE FROM oauth_grants WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("remove grants: %w", err)
	}
	return nil
}
