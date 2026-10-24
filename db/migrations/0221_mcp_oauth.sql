-- Sign-in for AI assistants that connect over MCP with OAuth (Home Assistant
-- and others that find the sign-in by themselves). A client is an app that may
-- ask for a sign-in; a grant is one administrator saying yes to one client; a
-- token is an access or refresh token of a grant. Tokens are random values
-- that are only stored as a SHA-256 hash.
CREATE TABLE oauth_clients (
    client_id     TEXT    PRIMARY KEY,
    secret_hash   TEXT    NOT NULL DEFAULT '',   -- '' for a public client
    name          TEXT    NOT NULL,
    redirect_uris TEXT    NOT NULL,              -- JSON list
    preregistered INTEGER NOT NULL DEFAULT 0,    -- 1 when an administrator made it by hand
    created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE oauth_grants (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id    TEXT    NOT NULL,
    user_id      INTEGER NOT NULL,
    scope        TEXT    NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT    NOT NULL DEFAULT '',
    revoked_at   TEXT
);
CREATE INDEX oauth_grants_user ON oauth_grants (user_id);
CREATE INDEX oauth_grants_client ON oauth_grants (client_id);

CREATE TABLE oauth_tokens (
    hash       TEXT    PRIMARY KEY,
    grant_id   INTEGER NOT NULL,
    kind       TEXT    NOT NULL,                 -- 'access' or 'refresh'
    expires_at TEXT    NOT NULL,
    used_at    TEXT    NOT NULL DEFAULT ''       -- a refresh token is used once
);
CREATE INDEX oauth_tokens_grant ON oauth_tokens (grant_id);
