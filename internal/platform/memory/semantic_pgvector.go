package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
)

// SemanticMemoryPgVector stores semantic-tier entries in a PostgreSQL table
// with a pgvector `vector(dim)` column. Recall is true similarity search via
// cosine distance (`<=>` operator from pgvector) rather than the keyword
// fallback used by the SQLite implementation.
//
// The backend manages its own table — `memory_semantic_vec` — so the standard
// `memory_semantic` table (driven by GORM AutoMigrate) is left untouched and
// SQLite + the keyword fallback continue to work unchanged.
type SemanticMemoryPgVector struct {
	db         *sql.DB
	dim        int
	tableName  string
}

// NewSemanticMemoryPgVector wires the backend against an already-open
// *gorm.DB (a postgres connection). It ensures the `vector` extension exists
// and the backing table is created. dim is the embedding dimensionality
// (typically 1536 for OpenAI-class models).
func NewSemanticMemoryPgVector(ctx context.Context, gdb *gorm.DB, dim int) (*SemanticMemoryPgVector, error) {
	if dim <= 0 {
		dim = 1536
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("pgvector: extract sql.DB: %w", err)
	}
	m := &SemanticMemoryPgVector{db: sqlDB, dim: dim, tableName: "memory_semantic_vec"}
	if err := m.ensureSchema(ctx); err != nil {
		return nil, fmt.Errorf("pgvector: schema: %w", err)
	}
	return m, nil
}

func (m *SemanticMemoryPgVector) ensureSchema(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return err
	}
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id           TEXT PRIMARY KEY,
		agent_id     TEXT NOT NULL,
		twin_id      TEXT NOT NULL,
		domain       TEXT NOT NULL DEFAULT '',
		content      TEXT NOT NULL,
		embedding    vector(%d),
		confidence   DOUBLE PRECISION NOT NULL DEFAULT 1.0,
		sources_json TEXT NOT NULL DEFAULT '[]',
		created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at   TIMESTAMPTZ
	)`, m.tableName, m.dim)
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return err
	}
	// Index for cosine-distance recall; ivfflat needs a small number of rows
	// inserted before it's worth building, so we skip it here and let the
	// operator add it manually once volumes warrant it.
	if _, err := m.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS memory_semantic_vec_agent_idx ON `+m.tableName+` (agent_id)`,
	); err != nil {
		return err
	}
	return nil
}

func (m *SemanticMemoryPgVector) Store(ctx context.Context, e memory.Entry) error {
	if e.ID == "" {
		e.ID = fmt.Sprintf("smv-%d", time.Now().UnixNano())
	}
	e.Tier = string(memory.TierSemantic)
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	srcJ, _ := json.Marshal(e.Sources)
	embedding := embeddingLiteral(e.Embedding, m.dim)

	stmt := fmt.Sprintf(`INSERT INTO %s
		(id, agent_id, twin_id, domain, content, embedding, confidence, sources_json, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, %s, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			content      = EXCLUDED.content,
			embedding    = EXCLUDED.embedding,
			confidence   = EXCLUDED.confidence,
			sources_json = EXCLUDED.sources_json,
			expires_at   = EXCLUDED.expires_at`, m.tableName, embedding)

	_, err := m.db.ExecContext(ctx, stmt,
		e.ID, string(e.AgentID), e.TwinID, e.Domain, e.Content,
		e.Confidence, string(srcJ), e.CreatedAt, e.ExpiresAt,
	)
	return err
}

func (m *SemanticMemoryPgVector) Recall(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	var (
		where []string
		args  []any
	)
	if q.AgentID != "" {
		where = append(where, fmt.Sprintf("agent_id = $%d", len(args)+1))
		args = append(args, string(q.AgentID))
	}
	if q.TwinID != "" {
		where = append(where, fmt.Sprintf("twin_id = $%d", len(args)+1))
		args = append(args, q.TwinID)
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Order by cosine distance to the query embedding when supplied; otherwise
	// fall back to recency. This keeps the recall path useful even before the
	// embedding pipeline is wired.
	orderBy := "created_at DESC"
	if len(q.Embedding) > 0 {
		orderBy = fmt.Sprintf("embedding <=> %s ASC", embeddingLiteral(q.Embedding, m.dim))
	}

	limit := q.TopK
	if limit <= 0 {
		limit = 5
	}

	stmt := fmt.Sprintf(`SELECT id, agent_id, twin_id, domain, content, confidence, sources_json, created_at, expires_at
		FROM %s %s ORDER BY %s LIMIT %d`, m.tableName, whereClause, orderBy, limit)

	rows, err := m.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []memory.Entry
	for rows.Next() {
		var (
			id, agentID, twinID, domain, content, srcJ string
			confidence                                 float64
			createdAt                                  time.Time
			expiresAt                                  sql.NullTime
		)
		if err := rows.Scan(&id, &agentID, &twinID, &domain, &content, &confidence, &srcJ, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		var sources []string
		_ = json.Unmarshal([]byte(srcJ), &sources)
		e := memory.Entry{
			ID: id, AgentID: coreagent.AgentID(agentID), TwinID: twinID, Tier: string(memory.TierSemantic),
			Domain: domain, Content: content, Confidence: confidence, Sources: sources,
			CreatedAt: createdAt,
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			e.ExpiresAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (m *SemanticMemoryPgVector) Forget(ctx context.Context, p memory.RetentionPolicy) error {
	if p.Before == nil && p.MinScore <= 0 {
		return nil
	}
	var (
		where []string
		args  []any
	)
	// Age and confidence are OR'd: an entry too old OR with too low a score
	// is purged. The whole filter is AND'd with agent/twin scoping so the
	// caller can confine the sweep to a single tenant.
	var orParts []string
	if p.Before != nil {
		orParts = append(orParts, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *p.Before)
	}
	if p.MinScore > 0 {
		orParts = append(orParts, fmt.Sprintf("confidence < $%d", len(args)+1))
		args = append(args, p.MinScore)
	}
	where = append(where, "("+strings.Join(orParts, " OR ")+")")
	if p.AgentID != "" {
		where = append(where, fmt.Sprintf("agent_id = $%d", len(args)+1))
		args = append(args, string(p.AgentID))
	}
	if p.TwinID != "" {
		where = append(where, fmt.Sprintf("twin_id = $%d", len(args)+1))
		args = append(args, p.TwinID)
	}
	stmt := fmt.Sprintf(`DELETE FROM %s WHERE %s`, m.tableName, strings.Join(where, " AND "))
	_, err := m.db.ExecContext(ctx, stmt, args...)
	return err
}

func (m *SemanticMemoryPgVector) Consolidate(_ context.Context, _ coreagent.AgentID) error {
	return nil
}

// embeddingLiteral renders a []float32 as a pgvector literal like
// '[1.0,2.0,3.0]'::vector. When emb is empty or wrong-sized, returns NULL so
// the column stores NULL rather than a malformed vector.
func embeddingLiteral(emb []float32, dim int) string {
	if len(emb) == 0 || len(emb) != dim {
		return "NULL"
	}
	var sb strings.Builder
	sb.WriteString("'[")
	for i, v := range emb {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", v)
	}
	sb.WriteString("]'::vector")
	return sb.String()
}
