package domain

import (
	nanoid "github.com/matoous/go-nanoid/v2"
)

const (
	// VersionIDAlphabet uses a URL-safe, human-readable alphabet without ambiguous chars.
	VersionIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	// VersionIDLength produces IDs like "ver_k8f2m9x1p3" — readable and collision-resistant.
	VersionIDLength = 12
	// VersionIDPrefix makes version IDs instantly recognizable.
	VersionIDPrefix = "ver_"
)

// nanoidGenerate is the ID generation function, replaceable in tests.
var nanoidGenerate = nanoid.Generate

// NewVersionID generates a human-readable, unique version identifier.
func NewVersionID() string {
	id, err := nanoidGenerate(VersionIDAlphabet, VersionIDLength)
	if err != nil {
		// nanoidGenerate only errors on invalid params; our constants are valid.
		panic("nanoid generation failed: " + err.Error())
	}
	return VersionIDPrefix + id
}
