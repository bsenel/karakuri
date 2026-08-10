// Package sql provides a database/sql-backed implementation of auth.Store and
// auth.CredentialStore.
//
// It deliberately takes no ORM and no driver dependency: the caller opens its
// own *sql.DB with whatever driver it already uses, and this package speaks
// plain SQL against it. Only two dialects differ in ways that matter —
// placeholder syntax and a handful of column types — and both are handled by
// the Dialect switch rather than by separate implementations.
package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bsenel/karakuri/auth"
)

// Dialect selects the SQL flavour. Anything else is rejected at construction
// rather than producing statements that fail at the first query.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// ErrUnsupportedDialect is returned by New for an unrecognised dialect.
var ErrUnsupportedDialect = errors.New("auth/sql: unsupported dialect")

// Options configures a Store.
type Options struct {
	// Dialect defaults to SQLite.
	Dialect Dialect

	// TablePrefix namespaces the tables, e.g. "karakuri_" yields
	// "karakuri_auth_principals". Defaults to none.
	TablePrefix string
}

// Store implements auth.Store and auth.CredentialStore over database/sql.
type Store struct {
	db      *sql.DB
	dialect Dialect
	prefix  string
}

var (
	_ auth.Store           = (*Store)(nil)
	_ auth.CredentialStore = (*Store)(nil)
)

// New wraps an open database handle. The caller keeps ownership of db —
// Store never closes it.
func New(db *sql.DB, opts Options) (*Store, error) {
	if db == nil {
		return nil, errors.New("auth/sql: nil *sql.DB")
	}
	dialect := opts.Dialect
	if dialect == "" {
		dialect = SQLite
	}
	if dialect != SQLite && dialect != Postgres {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
	return &Store{db: db, dialect: dialect, prefix: opts.TablePrefix}, nil
}

// table returns a prefixed table name.
func (s *Store) table(name string) string { return s.prefix + name }

// rebind converts "?" placeholders to the dialect's own form. This is the only
// dialect-specific code on the query path.
func (s *Store) rebind(query string) string {
	if s.dialect != Postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Store) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.rebind(q), args...)
}

func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.rebind(q), args...)
}

func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.rebind(q), args...)
}

// ── Principals ──────────────────────────────────────────────────────────────

func (s *Store) GetPrincipal(ctx context.Context, id string) (auth.Principal, error) {
	row := s.queryRow(ctx,
		`SELECT id, name, kind, attrs_json, disabled FROM `+s.table("auth_principals")+` WHERE id = ?`, id)
	p, err := scanPrincipal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Principal{}, fmt.Errorf("%w: %q", auth.ErrPrincipalNotFound, id)
	}
	return p, err
}

func (s *Store) ListPrincipals(ctx context.Context) ([]auth.Principal, error) {
	rows, err := s.query(ctx,
		`SELECT id, name, kind, attrs_json, disabled FROM `+s.table("auth_principals")+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []auth.Principal
	for rows.Next() {
		p, err := scanPrincipal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PutPrincipal(ctx context.Context, p auth.Principal) error {
	if p.ID == "" {
		return fmt.Errorf("auth/sql: principal ID is required")
	}
	attrs, err := marshalJSON(p.Attrs, "{}")
	if err != nil {
		return err
	}
	_, err = s.exec(ctx,
		`INSERT INTO `+s.table("auth_principals")+` (id, name, kind, attrs_json, disabled)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind,
		   attrs_json = EXCLUDED.attrs_json, disabled = EXCLUDED.disabled`,
		p.ID, p.Name, string(p.Kind), attrs, p.Disabled)
	return err
}

func (s *Store) DeletePrincipal(ctx context.Context, id string) error {
	// Nothing outlives a deleted principal — grants, credentials and tokens all
	// go, or a re-created ID would silently inherit them.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM `+s.table("auth_principals")+` WHERE id = ?`), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", auth.ErrPrincipalNotFound, id)
	}
	for _, stmt := range []string{
		`DELETE FROM ` + s.table("auth_role_bindings") + ` WHERE principal_id = ?`,
		`DELETE FROM ` + s.table("auth_credentials") + ` WHERE principal_id = ?`,
		`DELETE FROM ` + s.table("auth_refresh_tokens") + ` WHERE principal_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, s.rebind(stmt), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type rowScanner interface{ Scan(dest ...any) error }

func scanPrincipal(row rowScanner) (auth.Principal, error) {
	var (
		p     auth.Principal
		kind  string
		attrs string
	)
	if err := row.Scan(&p.ID, &p.Name, &kind, &attrs, &p.Disabled); err != nil {
		return auth.Principal{}, err
	}
	p.Kind = auth.Kind(kind)
	if err := unmarshalJSON(attrs, &p.Attrs); err != nil {
		return auth.Principal{}, err
	}
	return p, nil
}

// ── Roles and policies ──────────────────────────────────────────────────────

func (s *Store) GetRole(ctx context.Context, name string) (auth.Role, error) {
	row := s.queryRow(ctx,
		`SELECT name, description, inherits_json, system FROM `+s.table("auth_roles")+` WHERE name = ?`, name)
	r, err := scanRole(row)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Role{}, fmt.Errorf("%w: %q", auth.ErrRoleNotFound, name)
	}
	if err != nil {
		return auth.Role{}, err
	}
	policies, err := s.policiesForRoles(ctx, name)
	if err != nil {
		return auth.Role{}, err
	}
	r.Policies = policies[name]
	return r, nil
}

func (s *Store) ListRoles(ctx context.Context) ([]auth.Role, error) {
	rows, err := s.query(ctx,
		`SELECT name, description, inherits_json, system FROM `+s.table("auth_roles")+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []auth.Role
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// One extra query for every role's policies rather than one per role.
	policies, err := s.policiesForRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Policies = policies[out[i].Name]
	}
	return out, nil
}

func (s *Store) PutRole(ctx context.Context, r auth.Role) error {
	if r.Name == "" {
		return fmt.Errorf("auth/sql: role name is required")
	}
	existing, err := s.GetRole(ctx, r.Name)
	switch {
	case err == nil && existing.System && !r.System:
		// System roles are immutable: an operator editing "admin" is how
		// everyone gets locked out.
		return fmt.Errorf("%w: %q", auth.ErrSystemRole, r.Name)
	case err != nil && !errors.Is(err, auth.ErrRoleNotFound):
		return err
	}

	inherits, err := marshalJSON(r.Inherits, "[]")
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.rebind(
		`INSERT INTO `+s.table("auth_roles")+` (name, description, inherits_json, system)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description,
		   inherits_json = EXCLUDED.inherits_json, system = EXCLUDED.system`),
		r.Name, r.Description, inherits, r.System); err != nil {
		return err
	}
	// Policies are replaced wholesale: a role's policy set is the unit of
	// change, and diffing it would only invite partial updates.
	if _, err := tx.ExecContext(ctx, s.rebind(
		`DELETE FROM `+s.table("auth_policies")+` WHERE role_name = ?`), r.Name); err != nil {
		return err
	}
	for i, p := range r.Policies {
		conds, err := marshalJSON(p.Conditions, "[]")
		if err != nil {
			return err
		}
		id := p.ID
		if id == "" {
			id = fmt.Sprintf("%s#%d", r.Name, i)
		}
		if _, err := tx.ExecContext(ctx, s.rebind(
			`INSERT INTO `+s.table("auth_policies")+` (id, role_name, ordinal, action, resource, effect, conditions_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`),
			id, r.Name, i, string(p.Action), p.Resource, string(p.Effect), conds); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteRole(ctx context.Context, name string) error {
	r, err := s.GetRole(ctx, name)
	if err != nil {
		return err
	}
	if r.System {
		return fmt.Errorf("%w: %q", auth.ErrSystemRole, name)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM `+s.table("auth_policies")+` WHERE role_name = ?`), name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(`DELETE FROM `+s.table("auth_roles")+` WHERE name = ?`), name); err != nil {
		return err
	}
	return tx.Commit()
}

// policiesForRoles loads policies for the named roles, or for every role when
// no names are given. Ordinal preserves the order they were written in, which
// is the order they appear in a role definition.
func (s *Store) policiesForRoles(ctx context.Context, names ...string) (map[string][]auth.Policy, error) {
	q := `SELECT id, role_name, action, resource, effect, conditions_json FROM ` + s.table("auth_policies")
	args := make([]any, 0, len(names))
	if len(names) > 0 {
		placeholders := make([]string, len(names))
		for i, n := range names {
			placeholders[i] = "?"
			args = append(args, n)
		}
		q += ` WHERE role_name IN (` + strings.Join(placeholders, ", ") + `)`
	}
	q += ` ORDER BY role_name, ordinal`

	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]auth.Policy{}
	for rows.Next() {
		var (
			p      auth.Policy
			role   string
			action string
			effect string
			conds  string
		)
		if err := rows.Scan(&p.ID, &role, &action, &p.Resource, &effect, &conds); err != nil {
			return nil, err
		}
		p.Action = auth.Action(action)
		p.Effect = auth.Effect(effect)
		if err := unmarshalJSON(conds, &p.Conditions); err != nil {
			return nil, err
		}
		out[role] = append(out[role], p)
	}
	return out, rows.Err()
}

func scanRole(row rowScanner) (auth.Role, error) {
	var (
		r        auth.Role
		inherits string
	)
	if err := row.Scan(&r.Name, &r.Description, &inherits, &r.System); err != nil {
		return auth.Role{}, err
	}
	if err := unmarshalJSON(inherits, &r.Inherits); err != nil {
		return auth.Role{}, err
	}
	return r, nil
}

// ── Role bindings ───────────────────────────────────────────────────────────

func (s *Store) ListBindings(ctx context.Context, principalID string) ([]auth.RoleBinding, error) {
	q := `SELECT id, principal_id, role, scope FROM ` + s.table("auth_role_bindings")
	var args []any
	if principalID != "" {
		q += ` WHERE principal_id = ?`
		args = append(args, principalID)
	}
	q += ` ORDER BY id`

	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []auth.RoleBinding
	for rows.Next() {
		var b auth.RoleBinding
		if err := rows.Scan(&b.ID, &b.PrincipalID, &b.Role, &b.Scope); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) PutBinding(ctx context.Context, b auth.RoleBinding) error {
	if b.ID == "" || b.PrincipalID == "" || b.Role == "" {
		return fmt.Errorf("auth/sql: binding needs an ID, principal and role")
	}
	_, err := s.exec(ctx,
		`INSERT INTO `+s.table("auth_role_bindings")+` (id, principal_id, role, scope)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET principal_id = EXCLUDED.principal_id,
		   role = EXCLUDED.role, scope = EXCLUDED.scope`,
		b.ID, b.PrincipalID, b.Role, b.EffectiveScope())
	return err
}

func (s *Store) DeleteBinding(ctx context.Context, id string) error {
	res, err := s.exec(ctx, `DELETE FROM `+s.table("auth_role_bindings")+` WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", auth.ErrBindingNotFound, id)
	}
	return nil
}

// ── JSON and time helpers ───────────────────────────────────────────────────

func marshalJSON(v any, empty string) (string, error) {
	if v == nil {
		return empty, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if s := string(b); s != "null" {
		return s, nil
	}
	return empty, nil
}

func unmarshalJSON(s string, dest any) error {
	if s == "" || s == "null" {
		return nil
	}
	return json.Unmarshal([]byte(s), dest)
}

// Timestamps are stored as epoch milliseconds rather than as native datetime
// columns. Drivers disagree about how DATETIME round-trips (text vs numeric,
// timezone handling, sub-second precision), and this store has to behave
// identically on SQLite and Postgres; an integer has no such ambiguity.
func toMillis(t time.Time) int64 { return t.UTC().UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func nullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: toMillis(*t), Valid: true}
}

func millisPtr(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := fromMillis(n.Int64)
	return &t
}
