package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/auth"
)

// ── Credentials ─────────────────────────────────────────────────────────────

func (s *Store) GetCredential(ctx context.Context, principalID string) (auth.Credential, error) {
	var (
		c         auth.Credential
		updatedAt int64
	)
	err := s.queryRow(ctx,
		`SELECT principal_id, password_hash, updated_at FROM `+s.table("auth_credentials")+` WHERE principal_id = ?`,
		principalID).Scan(&c.PrincipalID, &c.PasswordHash, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Credential{}, fmt.Errorf("%w: %q", auth.ErrCredentialNotFound, principalID)
	}
	if err != nil {
		return auth.Credential{}, err
	}
	c.UpdatedAt = fromMillis(updatedAt)
	return c, nil
}

func (s *Store) PutCredential(ctx context.Context, c auth.Credential) error {
	if c.PrincipalID == "" {
		return fmt.Errorf("auth/sql: credential needs a principal ID")
	}
	updatedAt := c.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	_, err := s.exec(ctx,
		`INSERT INTO `+s.table("auth_credentials")+` (principal_id, password_hash, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (principal_id) DO UPDATE SET password_hash = EXCLUDED.password_hash,
		   updated_at = EXCLUDED.updated_at`,
		c.PrincipalID, c.PasswordHash, toMillis(updatedAt))
	return err
}

func (s *Store) DeleteCredential(ctx context.Context, principalID string) error {
	res, err := s.exec(ctx, `DELETE FROM `+s.table("auth_credentials")+` WHERE principal_id = ?`, principalID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", auth.ErrCredentialNotFound, principalID)
	}
	return nil
}

// ── Refresh tokens ──────────────────────────────────────────────────────────

func (s *Store) PutRefreshToken(ctx context.Context, t auth.RefreshToken) error {
	if t.ID == "" || t.TokenHash == "" || t.PrincipalID == "" || t.FamilyID == "" {
		return fmt.Errorf("auth/sql: refresh token needs an ID, hash, family and principal")
	}
	_, err := s.exec(ctx,
		`INSERT INTO `+s.table("auth_refresh_tokens")+`
		   (id, family_id, parent_id, principal_id, token_hash, issued_at, expires_at, used_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.FamilyID, t.ParentID, t.PrincipalID, t.TokenHash,
		toMillis(t.IssuedAt), toMillis(t.ExpiresAt), nullMillis(t.UsedAt), nullMillis(t.RevokedAt))
	return err
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, hash string) (auth.RefreshToken, error) {
	var (
		t                   auth.RefreshToken
		issuedAt, expiresAt int64
		usedAt, revokedAt   sql.NullInt64
	)
	err := s.queryRow(ctx,
		`SELECT id, family_id, parent_id, principal_id, token_hash, issued_at, expires_at, used_at, revoked_at
		 FROM `+s.table("auth_refresh_tokens")+` WHERE token_hash = ?`, hash).
		Scan(&t.ID, &t.FamilyID, &t.ParentID, &t.PrincipalID, &t.TokenHash,
			&issuedAt, &expiresAt, &usedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}
	if err != nil {
		return auth.RefreshToken{}, err
	}
	t.IssuedAt = fromMillis(issuedAt)
	t.ExpiresAt = fromMillis(expiresAt)
	t.UsedAt = millisPtr(usedAt)
	t.RevokedAt = millisPtr(revokedAt)
	return t, nil
}

// SpendRefreshToken marks a token used, but only if it has not been used
// already. The "used_at IS NULL" predicate is what makes rotation safe under
// concurrency: two clients exchanging the same token race on this one
// statement, and the database decides the winner. Reporting false lets the
// caller treat the loser exactly like a replay.
func (s *Store) SpendRefreshToken(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := s.exec(ctx,
		`UPDATE `+s.table("auth_refresh_tokens")+` SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		toMillis(at), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// No row updated: either the token is gone, or it was already spent.
	var exists int
	err = s.queryRow(ctx, `SELECT COUNT(1) FROM `+s.table("auth_refresh_tokens")+` WHERE id = ?`, id).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists == 0 {
		return false, auth.ErrInvalidRefreshToken
	}
	return false, nil
}

func (s *Store) RevokeRefreshFamily(ctx context.Context, familyID string, at time.Time) error {
	_, err := s.exec(ctx,
		`UPDATE `+s.table("auth_refresh_tokens")+` SET revoked_at = ? WHERE family_id = ? AND revoked_at IS NULL`,
		toMillis(at), familyID)
	return err
}

func (s *Store) RevokeRefreshTokensForPrincipal(ctx context.Context, principalID string, at time.Time) error {
	_, err := s.exec(ctx,
		`UPDATE `+s.table("auth_refresh_tokens")+` SET revoked_at = ? WHERE principal_id = ? AND revoked_at IS NULL`,
		toMillis(at), principalID)
	return err
}

// DeleteExpiredRefreshTokens removes tokens that expired before the cutoff. It
// is not part of the auth.CredentialStore interface — rotation means the table
// grows one row per exchange, so a deployment wants a periodic sweep, but when
// and how often is an operator's decision.
func (s *Store) DeleteExpiredRefreshTokens(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.exec(ctx,
		`DELETE FROM `+s.table("auth_refresh_tokens")+` WHERE expires_at < ?`, toMillis(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
