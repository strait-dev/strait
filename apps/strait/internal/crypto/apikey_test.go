package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAPIKey(t *testing.T) {
	t.Parallel()
	key := "strait_abc123"
	sum := sha256.Sum256([]byte(key))
	require.Equal(t, hex.EncodeToString(sum[:]), HashAPIKey(key))
	require.NotEqual(t, HashAPIKey("a"), HashAPIKey("b"))
	require.Len(t, HashAPIKey(key), 64)
}
