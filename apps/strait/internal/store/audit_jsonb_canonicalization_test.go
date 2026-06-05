package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJSONBDBTX models a minimal Postgres-compatible DB that reproduces
// JSONB text canonicalization on round-trip. It is deliberately NOT a
// TxBeginner so withTxInheritKeys runs the CreateAuditEvent body inline
// against this fake — sufficient to exercise the sign + verify loop end
// to end without Docker.
//
// When the INSERT fires, the fake stores the row in rows with the details
// value REPLACED by canonicalization(input) — mimicking what Postgres
// does when it parses JSONB text and renders it back out with inserted
// whitespace after colons and commas. SELECT returns that canonical
// form. If writer and verifier compute HMAC over different byte forms,
// the verifier's signature mismatches and the test fails.
type fakeJSONBDBTX struct {
	rows       []map[string]any
	signingKey []byte
}

// canonicalizeJSONB reproduces Postgres's JSONB text output: parse the
// raw bytes into map/slice form, then re-marshal with the stdlib
// encoder (which inserts a space after every colon when
// json.Indent-style output is used). Postgres JSONB goes further than
// this in key ordering and number formatting — reproducing it exactly
// is not the goal. The goal is to ensure ANY whitespace or ordering
// divergence surfaces as a HMAC mismatch, so even this partial
// canonicalization is enough to tell the two forms apart.
func canonicalizeJSONB(raw []byte) []byte {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// Malformed input would fail the real DB too; fall through.
		return raw
	}
	// Re-marshal the parsed structure. json.Marshal produces
	// whitespace-free output, whereas Postgres JSONB text output adds
	// spaces. We deliberately walk the map and re-emit with a space
	// after every colon to simulate Postgres's "{"k": v}" shape.
	return appendSpaceAfterColons(mustMarshalDeterministic(parsed))
}

// mustMarshalDeterministic re-marshals with sorted map keys to keep
// the output predictable in tests. json.Marshal already sorts map keys
// lexicographically, so this is equivalent; we call it out so future
// readers understand the contract.
func mustMarshalDeterministic(v any) []byte {
	out, _ := json.Marshal(v)
	return out
}

// appendSpaceAfterColons walks the marshaled output and injects a space
// after every unquoted ':' and ','. This simulates Postgres's JSONB text
// renderer, which writes "{"k": 1, "b": 2}" rather than the compact
// "{"k":1,"b":2}" that json.Marshal emits.
func appendSpaceAfterColons(raw []byte) []byte {
	var buf []byte
	inString := false
	escape := false
	for _, b := range raw {
		buf = append(buf, b)
		if escape {
			escape = false
			continue
		}
		if b == '\\' && inString {
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if b == ':' || b == ',' {
			buf = append(buf, ' ')
		}
	}
	return buf
}

func (f *fakeJSONBDBTX) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "pg_advisory_xact_lock"):
		return pgconn.CommandTag{}, nil
	case strings.Contains(sql, "UPDATE audit_events SET signature"):
		if len(args) < 2 {
			return pgconn.CommandTag{}, errors.New("fake: UPDATE expects (signature, id)")
		}
		sig, ok1 := args[0].(string)
		id, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return pgconn.CommandTag{}, errors.New("fake: UPDATE args wrong types")
		}
		for _, row := range f.rows {
			if row["id"] == id {
				row["signature"] = sig
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
		}
		return pgconn.CommandTag{}, errors.New("fake: UPDATE row not found")
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeJSONBDBTX) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "FROM audit_events") {
		return nil, errors.New("fake: unexpected Query")
	}
	if len(args) < 1 {
		return nil, errors.New("fake: Query missing project_id")
	}
	projectID, ok := args[0].(string)
	if !ok {
		return nil, errors.New("fake: Query project_id not a string")
	}
	if strings.Contains(sql, "DISTINCT rotation_epoch") {
		epochs := make(map[int]bool)
		for _, row := range f.rows {
			if row["project_id"] == projectID {
				epochs[row["rotation_epoch"].(int)] = true
			}
		}
		var epochRows []map[string]any
		for e := range epochs {
			epochRows = append(epochRows, map[string]any{"epoch": e})
		}
		return &fakeEpochRows{rows: epochRows}, nil
	}
	matched := make([]map[string]any, 0, len(f.rows))
	for _, row := range f.rows {
		if row["project_id"] == projectID {
			matched = append(matched, row)
		}
	}
	return &fakeJSONBRows{rows: matched}, nil
}

func (f *fakeJSONBDBTX) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "pg_try_advisory_xact_lock"):
		return &fakeScalarRow{val: true}
	case strings.Contains(sql, "MAX(rotation_epoch)") && strings.Contains(sql, "FROM audit_events"):
		projectID, _ := args[0].(string)
		var maxEpoch int
		for _, row := range f.rows {
			if row["project_id"] == projectID {
				if epoch, _ := row["rotation_epoch"].(int); epoch > maxEpoch {
					maxEpoch = epoch
				}
			}
		}
		return &fakeScalarRow{val: maxEpoch}
	case strings.Contains(sql, "SELECT COALESCE(") && strings.Contains(sql, "FROM audit_events"):
		// Tail read for previous_hash. Real query takes (projectID,
		// shardID, ZeroHash) so the tail is shard-scoped — legacy ('')
		// writers never chain from a sharded row and vice versa.
		projectID, _ := args[0].(string)
		shardID, _ := args[1].(string)
		zeroHash, _ := args[2].(string)
		var tail string
		for _, row := range f.rows {
			if row["project_id"] == projectID && row["shard_id"] == shardID && row["signature"] != "" {
				tail = row["signature"].(string)
			}
		}
		if tail == "" {
			tail = zeroHash
		}
		return &fakeJSONBRow{tail: tail}
	case strings.Contains(sql, "INSERT INTO audit_events") && strings.Contains(sql, "RETURNING details"):
		// Insert with RETURNING details. Positional args from the real
		// CreateAuditEvent: (id, project_id, actor_id, actor_type,
		// action, resource_type, resource_id, details, previous_hash,
		// created_at, remote_ip, user_agent, request_id, trace_id,
		// schema_version, is_anchor, rotation_epoch, shard_id).
		if len(args) < 18 {
			return &fakeJSONBRow{err: errors.New("fake: INSERT wrong arity")}
		}
		detailsRaw, _ := args[7].(json.RawMessage)
		canonical := canonicalizeJSONB(detailsRaw)
		row := map[string]any{
			"id":             args[0],
			"project_id":     args[1],
			"actor_id":       args[2],
			"actor_type":     args[3],
			"action":         args[4],
			"resource_type":  args[5],
			"resource_id":    args[6],
			"details":        json.RawMessage(canonical),
			"signature":      "",
			"previous_hash":  args[8],
			"created_at":     args[9],
			"remote_ip":      args[10],
			"user_agent":     args[11],
			"request_id":     args[12],
			"trace_id":       args[13],
			"schema_version": args[14],
			"is_anchor":      args[15],
			"rotation_epoch": args[16],
			"shard_id":       args[17],
		}
		f.rows = append(f.rows, row)
		return &fakeJSONBRow{returningDetails: json.RawMessage(canonical)}
	case strings.Contains(sql, "FROM audit_signing_keys"):
		// No stored per-epoch key — the test uses the global fallback.
		return &fakeJSONBRow{err: pgx.ErrNoRows}
	}
	return &fakeJSONBRow{err: errors.New("fake: unexpected QueryRow")}
}

// fakeJSONBRow returns either a tail signature, a returned details blob,
// or an error depending on which call it is for.
type fakeJSONBRow struct {
	tail             string
	returningDetails json.RawMessage
	err              error
}

func (r *fakeJSONBRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.tail != "" && len(dest) == 1 {
		*dest[0].(*string) = r.tail
		return nil
	}
	if r.returningDetails != nil && len(dest) == 1 {
		*dest[0].(*json.RawMessage) = r.returningDetails
		return nil
	}
	return errors.New("fake: Scan called without a configured response")
}

type fakeScalarRow struct{ val any }

func (r *fakeScalarRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("fakeScalarRow: expected exactly 1 dest")
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.val.(bool)
	case *string:
		*d = r.val.(string)
	case *int:
		*d = r.val.(int)
	default:
		return fmt.Errorf("fakeScalarRow: unsupported type %T", dest[0])
	}
	return nil
}

// fakeEpochRows returns a single int column per row for the DISTINCT
// rotation_epoch pre-load query used by VerifyAuditChain.
type fakeEpochRows struct {
	rows []map[string]any
	idx  int
}

func (r *fakeEpochRows) Close()                                       {}
func (r *fakeEpochRows) Err() error                                   { return nil }
func (r *fakeEpochRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeEpochRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeEpochRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeEpochRows) RawValues() [][]byte                          { return nil }
func (r *fakeEpochRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeEpochRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeEpochRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("fake: Scan without Next")
	}
	row := r.rows[r.idx-1]
	*dest[0].(*int) = row["epoch"].(int)
	return nil
}

// fakeJSONBRows replays stored rows through pgx.Rows.
type fakeJSONBRows struct {
	rows []map[string]any
	idx  int
}

func (r *fakeJSONBRows) Close()                                       {}
func (r *fakeJSONBRows) Err() error                                   { return nil }
func (r *fakeJSONBRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeJSONBRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeJSONBRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeJSONBRows) RawValues() [][]byte                          { return nil }
func (r *fakeJSONBRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeJSONBRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeJSONBRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("fake: Scan without Next")
	}
	row := r.rows[r.idx-1]
	// Order must match the SELECT in VerifyAuditChain.
	*dest[0].(*string) = row["id"].(string)
	*dest[1].(*string) = row["project_id"].(string)
	*dest[2].(*string) = row["actor_id"].(string)
	*dest[3].(*string) = row["actor_type"].(string)
	*dest[4].(*string) = row["action"].(string)
	*dest[5].(*string) = row["resource_type"].(string)
	*dest[6].(*string) = row["resource_id"].(string)
	*dest[7].(*json.RawMessage) = row["details"].(json.RawMessage)
	*dest[8].(*string) = row["signature"].(string)
	*dest[9].(*string) = row["previous_hash"].(string)
	*dest[10].(*time.Time) = row["created_at"].(time.Time)
	*dest[11].(*string) = row["remote_ip"].(string)
	*dest[12].(*string) = row["user_agent"].(string)
	*dest[13].(*string) = row["request_id"].(string)
	*dest[14].(*string) = row["trace_id"].(string)
	*dest[15].(*uint16) = row["schema_version"].(uint16)
	*dest[16].(*bool) = row["is_anchor"].(bool)
	*dest[17].(*int) = row["rotation_epoch"].(int)
	shardID, _ := row["shard_id"].(string)
	*dest[18].(*string) = shardID
	return nil
}

// TestCreateAuditEvent_SignAndVerify_SurviveJSONBCanonicalization is the
// regression test for the sign-vs-verify divergence in the atomic insert path.
// When CreateAuditEvent computed HMAC over the
// raw pre-insert bytes while Postgres stored a whitespace-normalized JSONB
// form, every VerifyAuditChain call that round-tripped through the DB
// surfaced "signature mismatch at event <uuid>".
//
// This test models the JSONB normalization in a fake DB and runs the
// sign path + verify path in the same Go process. It guarantees the two
// paths agree on canonical form without requiring testcontainers, so the
// CI signal fires on the next regression before it reaches the slow
// integration layer.
func TestCreateAuditEvent_SignAndVerify_SurviveJSONBCanonicalization(t *testing.T) {
	t.Parallel()

	fake := &fakeJSONBDBTX{}
	q := New(fake)
	key, err := DeriveAuditSigningKey("canonicalization-test-secret")
	require.NoError(t, err)

	q.SetAuditSigningKey(key)
	fake.signingKey = key

	ctx := context.Background()
	projectID := "proj-canonicalization"

	// Insert three events with multi-key detail bodies so the JSONB text
	// form genuinely diverges from the compact Go encoding. If the writer
	// signs over the compact bytes and the verifier reads canonical bytes,
	// the first event will fail signature check on VerifyAuditChain.
	events := []*domain.AuditEvent{
		{
			ProjectID:    projectID,
			ActorID:      "actor-a",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "j-1",
			Details:      json.RawMessage(`{"a":1,"b":"two","c":true}`),
		},
		{
			ProjectID:    projectID,
			ActorID:      "actor-b",
			ActorType:    "user",
			Action:       domain.AuditActionJobUpdated,
			ResourceType: "job",
			ResourceID:   "j-1",
			Details:      json.RawMessage(`{"nested":{"k":"v","n":42}}`),
		},
		{
			ProjectID:    projectID,
			ActorID:      "actor-c",
			ActorType:    "user",
			Action:       domain.AuditActionJobDeleted,
			ResourceType: "job",
			ResourceID:   "j-1",
			Details:      json.RawMessage(`{}`),
		},
	}

	for _, ev := range events {
		require.NoError(t, q.CreateAuditEvent(ctx,
			ev))
		require.NotEmpty(t, ev.Signature)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t,
		result.Valid)
	assert.Equal(t,
		len(events), result.
			EventsChecked,
	)
}
