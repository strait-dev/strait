package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const auditKeyRotationUnitEnvelope = "audit-key-rotation-unit-envelope"

type auditKeyRotationFakeDB struct {
	keys           map[int][]byte
	shards         []string
	maxEpoch       int
	queryErr       error
	shardScanErr   error
	shardRowsErr   error
	insertKeyErr   error
	insertEventErr error
	updateSigErr   error
	lockErr        error
	anchors        []domain.AuditEvent
}

func (db *auditKeyRotationFakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "INSERT INTO audit_signing_keys"):
		if db.insertKeyErr != nil {
			return pgconn.CommandTag{}, db.insertKeyErr
		}
		if db.keys == nil {
			db.keys = make(map[int][]byte)
		}
		epoch, ok := args[1].(int)
		if !ok {
			return pgconn.CommandTag{}, errors.New("fake audit key db: epoch arg is not int")
		}
		ciphertext, ok := args[2].([]byte)
		if !ok {
			return pgconn.CommandTag{}, errors.New("fake audit key db: key material arg is not []byte")
		}
		db.keys[epoch] = append([]byte(nil), ciphertext...)
		return pgconn.NewCommandTag("INSERT 1"), nil
	case strings.Contains(sql, "UPDATE audit_events SET signature"):
		if db.updateSigErr != nil {
			return pgconn.CommandTag{}, db.updateSigErr
		}
		if len(db.anchors) == 0 {
			return pgconn.CommandTag{}, errors.New("fake audit key db: update before insert")
		}
		signature, ok := args[0].(string)
		if !ok {
			return pgconn.CommandTag{}, errors.New("fake audit key db: signature arg is not string")
		}
		db.anchors[len(db.anchors)-1].Signature = signature
		return pgconn.NewCommandTag("UPDATE 1"), nil
	default:
		return pgconn.NewCommandTag("SELECT 1"), nil
	}
}

func (db *auditKeyRotationFakeDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if db.queryErr != nil {
		return nil, db.queryErr
	}
	if !strings.Contains(sql, "SELECT shard_id") {
		return nil, errors.New("fake audit key db: unexpected query")
	}
	scans := make([]func(dest ...any) error, 0, len(db.shards))
	for _, shard := range db.shards {
		scans = append(scans, func(dest ...any) error {
			if db.shardScanErr != nil {
				return db.shardScanErr
			}
			*(dest[0].(*string)) = shard
			return nil
		})
	}
	return &mockRows{scanFns: scans, err: db.shardRowsErr}, nil
}

func (db *auditKeyRotationFakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "pg_try_advisory_xact_lock"):
		return &mockRow{scanFn: func(dest ...any) error {
			if db.lockErr != nil {
				return db.lockErr
			}
			*(dest[0].(*bool)) = true
			return nil
		}}
	case strings.Contains(sql, "SELECT key_material") && strings.Contains(sql, "FROM audit_signing_keys"):
		return &mockRow{scanFn: func(dest ...any) error {
			epoch, _ := args[1].(int)
			ciphertext, ok := db.keys[epoch]
			if !ok {
				return pgx.ErrNoRows
			}
			*(dest[0].(*[]byte)) = append([]byte(nil), ciphertext...)
			return nil
		}}
	case strings.Contains(sql, "SELECT COALESCE(MAX(rotation_epoch), 0)") && strings.Contains(sql, "FROM audit_events"):
		return &mockRow{scanFn: func(dest ...any) error {
			if db.queryErr != nil {
				return db.queryErr
			}
			*(dest[0].(*int)) = db.maxEpoch
			return nil
		}}
	case strings.Contains(sql, "SELECT COALESCE(") && strings.Contains(sql, "FROM audit_events"):
		return &mockRow{scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = ZeroHash
			return nil
		}}
	case strings.Contains(sql, "INSERT INTO audit_events") && strings.Contains(sql, "RETURNING details"):
		return &mockRow{scanFn: func(dest ...any) error {
			if db.insertEventErr != nil {
				return db.insertEventErr
			}
			if len(args) < 18 {
				return errors.New("fake audit key db: audit insert wrong arity")
			}
			details, ok := args[7].(json.RawMessage)
			if !ok {
				return errors.New("fake audit key db: details arg is not json.RawMessage")
			}
			ev := domain.AuditEvent{
				ID:            args[0].(string),
				ProjectID:     args[1].(string),
				ActorID:       args[2].(string),
				ActorType:     args[3].(string),
				Action:        args[4].(string),
				ResourceType:  args[5].(string),
				ResourceID:    args[6].(string),
				Details:       append(json.RawMessage(nil), details...),
				PreviousHash:  args[8].(string),
				CreatedAt:     args[9].(time.Time),
				SchemaVersion: args[14].(uint16),
				IsAnchor:      args[15].(bool),
				RotationEpoch: args[16].(int),
				ShardID:       args[17].(string),
			}
			db.anchors = append(db.anchors, ev)
			*(dest[0].(*json.RawMessage)) = append(json.RawMessage(nil), details...)
			return nil
		}}
	default:
		return &mockRow{scanFn: func(...any) error {
			return errors.New("fake audit key db: unexpected query row")
		}}
	}
}

func TestAuditSigningKeyEpochDerivationUnit(t *testing.T) {
	t.Parallel()

	_, err := DeriveAuditSigningKeyForEpoch("", "project-1", 1)
	require.ErrorContains(t, err, "secret is empty")

	_, err = DeriveAuditSigningKeyForEpochFromRoot(nil, "project-1", 1)
	require.ErrorContains(t, err, "root key is empty")

	keyA, err := DeriveAuditSigningKeyForEpoch("internal-secret", "project-1", 1)
	require.NoError(t, err)
	keyB, err := DeriveAuditSigningKeyForEpoch("internal-secret", "project-1", 1)
	require.NoError(t, err)
	keyOtherEpoch, err := DeriveAuditSigningKeyForEpoch("internal-secret", "project-1", 2)
	require.NoError(t, err)
	keyOtherProject, err := DeriveAuditSigningKeyForEpoch("internal-secret", "project-2", 1)
	require.NoError(t, err)

	require.Len(t, keyA, 32)
	require.Equal(t, keyA, keyB)
	require.NotEqual(t, keyA, keyOtherEpoch)
	require.NotEqual(t, keyA, keyOtherProject)
}

func TestAuditKeyEnvelopeEncryptionUnit(t *testing.T) {
	t.Parallel()

	envelopeKey, err := deriveSecretKey(auditKeyRotationUnitEnvelope)
	require.NoError(t, err)
	plaintext := bytes.Repeat([]byte{7}, 32)

	ciphertext, err := encryptAuditKey(plaintext, envelopeKey)
	require.NoError(t, err)
	require.Greater(t, len(ciphertext), len(plaintext))

	got, err := decryptAuditKey(ciphertext, envelopeKey)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)

	_, err = encryptAuditKey(plaintext, envelopeKey[:31])
	require.ErrorContains(t, err, "envelope key must be 32 bytes")

	_, err = decryptAuditKey(ciphertext[:4], envelopeKey)
	require.ErrorContains(t, err, "ciphertext too short")

	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 0xff
	_, err = decryptAuditKey(tampered, envelopeKey)
	require.ErrorContains(t, err, "decrypt audit key")
}

func TestGetAuditSigningKeyUnit(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for missing row", func(t *testing.T) {
		t.Parallel()

		got, err := New(&auditKeyRotationFakeDB{}).GetAuditSigningKey(context.Background(), "project-1", 1)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("requires configured envelope key for stored ciphertext", func(t *testing.T) {
		t.Parallel()

		db := &auditKeyRotationFakeDB{keys: map[int][]byte{1: []byte("ciphertext")}}
		got, err := New(db).GetAuditSigningKey(context.Background(), "project-1", 1)
		require.ErrorContains(t, err, "secret encryption key is not configured")
		require.Nil(t, got)
	})

	t.Run("decrypts with primary envelope key", func(t *testing.T) {
		t.Parallel()

		envelopeKey, err := deriveSecretKey(auditKeyRotationUnitEnvelope)
		require.NoError(t, err)
		want := bytes.Repeat([]byte{3}, 32)
		ciphertext, err := encryptAuditKey(want, envelopeKey)
		require.NoError(t, err)

		q := New(&auditKeyRotationFakeDB{keys: map[int][]byte{2: ciphertext}})
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		got, err := q.GetAuditSigningKey(context.Background(), "project-1", 2)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("falls back to old envelope keys", func(t *testing.T) {
		t.Parallel()

		oldEnvelopeKey, err := deriveSecretKey("old-audit-key-envelope")
		require.NoError(t, err)
		want := bytes.Repeat([]byte{9}, 32)
		ciphertext, err := encryptAuditKey(want, oldEnvelopeKey)
		require.NoError(t, err)

		q := New(&auditKeyRotationFakeDB{keys: map[int][]byte{3: ciphertext}})
		q.SetSecretEncryptionKey("new-audit-key-envelope")
		q.SetOldSecretEncryptionKeys([]string{"old-audit-key-envelope"})
		got, err := q.GetAuditSigningKey(context.Background(), "project-1", 3)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("wraps decrypt failure", func(t *testing.T) {
		t.Parallel()

		q := New(&auditKeyRotationFakeDB{keys: map[int][]byte{4: []byte("too-short")}})
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		got, err := q.GetAuditSigningKey(context.Background(), "project-1", 4)
		require.ErrorContains(t, err, "get audit signing key: decrypt")
		require.Nil(t, got)
	})
}

func TestStoreAuditSigningKeyUnit(t *testing.T) {
	t.Parallel()

	t.Run("requires envelope key", func(t *testing.T) {
		t.Parallel()

		err := New(&auditKeyRotationFakeDB{}).storeAuditSigningKey(context.Background(), "project-1", 1, bytes.Repeat([]byte{1}, 32), "actor-1")
		require.ErrorContains(t, err, "envelope key")
	})

	t.Run("encrypts and inserts key material", func(t *testing.T) {
		t.Parallel()

		db := &auditKeyRotationFakeDB{}
		q := New(db)
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		want := bytes.Repeat([]byte{4}, 32)

		require.NoError(t, q.storeAuditSigningKey(context.Background(), "project-1", 7, want, "actor-1"))
		require.Contains(t, db.keys, 7)
		require.NotEqual(t, want, db.keys[7])

		envelopeKey, err := deriveSecretKey(auditKeyRotationUnitEnvelope)
		require.NoError(t, err)
		got, err := decryptAuditKey(db.keys[7], envelopeKey)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("wraps insert errors", func(t *testing.T) {
		t.Parallel()

		insertErr := errors.New("insert failed")
		q := New(&auditKeyRotationFakeDB{insertKeyErr: insertErr})
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		err := q.storeAuditSigningKey(context.Background(), "project-1", 1, bytes.Repeat([]byte{1}, 32), "actor-1")
		require.ErrorIs(t, err, insertErr)
		require.ErrorContains(t, err, "store audit signing key: insert")
	})
}

func TestRotateAuditSigningKeyUnit(t *testing.T) {
	t.Parallel()

	t.Run("validates project and envelope configuration", func(t *testing.T) {
		t.Parallel()

		q := New(&auditKeyRotationFakeDB{})
		epoch, err := q.RotateAuditSigningKey(context.Background(), "", "actor-1")
		require.ErrorContains(t, err, "project id is empty")
		require.Zero(t, epoch)

		epoch, err = q.RotateAuditSigningKey(context.Background(), "project-1", "actor-1")
		require.ErrorContains(t, err, "secret encryption key is not configured")
		require.Zero(t, epoch)
	})

	t.Run("returns context cancellation before attempting rotation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		q := New(&auditKeyRotationFakeDB{})
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		epoch, err := q.RotateAuditSigningKey(ctx, "project-1", "actor-1")
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, epoch)
	})

	t.Run("exhausts unique violation retry budget", func(t *testing.T) {
		t.Parallel()

		db := &auditKeyRotationFakeDB{lockErr: &pgconn.PgError{Code: "23505", Message: "duplicate anchor"}}
		q := New(db)
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		root, err := DeriveAuditSigningKey("audit-root")
		require.NoError(t, err)
		q.SetAuditSigningKey(root)

		epoch, err := q.RotateAuditSigningKey(context.Background(), "project-1", "actor-1")
		require.ErrorContains(t, err, "exhausted retries")
		require.ErrorContains(t, err, "duplicate anchor")
		require.Zero(t, epoch)
	})

	t.Run("emits anchors for existing shards", func(t *testing.T) {
		t.Parallel()

		db := &auditKeyRotationFakeDB{maxEpoch: 2, shards: []string{"", "tenant-a"}}
		q := New(db)
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		root, err := DeriveAuditSigningKey("audit-root")
		require.NoError(t, err)
		q.SetAuditSigningKey(root)

		epoch, err := q.RotateAuditSigningKey(context.Background(), "project-1", "actor-1")
		require.NoError(t, err)
		require.Equal(t, 3, epoch)
		require.Len(t, db.anchors, 2)
		require.Equal(t, "epoch-3", db.anchors[0].ResourceID)
		require.Equal(t, "epoch-3-tenant-a", db.anchors[1].ResourceID)
		require.Equal(t, domain.AuditActionKeyRotated, db.anchors[1].Action)
		require.True(t, db.anchors[1].IsAnchor)
		require.Equal(t, 3, db.anchors[1].RotationEpoch)

		var details map[string]any
		require.NoError(t, json.Unmarshal(db.anchors[1].Details, &details))
		previousEpoch, ok := details["previous_epoch"].(float64)
		require.True(t, ok)
		require.Equal(t, 2, int(previousEpoch))
		newEpoch, ok := details["new_epoch"].(float64)
		require.True(t, ok)
		require.Equal(t, 3, int(newEpoch))
		require.Equal(t, "actor-1", details["rotated_by"])
		require.Equal(t, "tenant-a", details["shard_id"])
	})

	t.Run("defaults to legacy shard when project has no shards", func(t *testing.T) {
		t.Parallel()

		db := &auditKeyRotationFakeDB{}
		q := New(db)
		q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
		root, err := DeriveAuditSigningKey("audit-root")
		require.NoError(t, err)
		q.SetAuditSigningKey(root)

		epoch, err := q.RotateAuditSigningKey(context.Background(), "project-1", "actor-1")
		require.NoError(t, err)
		require.Equal(t, 1, epoch)
		require.Len(t, db.anchors, 1)
		require.Empty(t, db.anchors[0].ShardID)
	})

	t.Run("wraps rotation step errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			db         *auditKeyRotationFakeDB
			wantString string
		}{
			{name: "lock", db: &auditKeyRotationFakeDB{lockErr: errors.New("lock failed")}, wantString: "acquire project rotation lock"},
			{name: "max epoch", db: &auditKeyRotationFakeDB{queryErr: errors.New("max failed")}, wantString: "read max epoch"},
			{name: "store key", db: &auditKeyRotationFakeDB{insertKeyErr: errors.New("insert key failed")}, wantString: "store audit signing key: insert"},
			{name: "read shards", db: &auditKeyRotationFakeDB{queryErr: errors.New("query failed")}, wantString: "read max epoch"},
			{name: "scan shard", db: &auditKeyRotationFakeDB{shards: []string{"tenant-a"}, shardScanErr: errors.New("scan failed")}, wantString: "scan shard"},
			{name: "rows shard", db: &auditKeyRotationFakeDB{shards: []string{"tenant-a"}, shardRowsErr: errors.New("rows failed")}, wantString: "rows err"},
			{name: "insert anchor", db: &auditKeyRotationFakeDB{insertEventErr: errors.New("insert event failed")}, wantString: "write anchor"},
			{name: "update anchor signature", db: &auditKeyRotationFakeDB{updateSigErr: errors.New("signature failed")}, wantString: "write anchor"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				q := New(tt.db)
				q.SetSecretEncryptionKey(auditKeyRotationUnitEnvelope)
				root, err := DeriveAuditSigningKey("audit-root")
				require.NoError(t, err)
				q.SetAuditSigningKey(root)
				epoch, err := q.RotateAuditSigningKey(context.Background(), "project-1", "actor-1")
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantString)
				require.Zero(t, epoch)
			})
		}
	})
}
