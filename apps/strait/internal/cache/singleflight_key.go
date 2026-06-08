package cache

import (
	"fmt"
	"strconv"
)

func tierSingleflightKey[K comparable](name string, key K, minVersion int64, versioned bool) string {
	if keyString, ok := any(key).(string); ok {
		return tierSingleflightKeyParts(name, keyString, minVersion, versioned)
	}
	return tierSingleflightKeyParts(name, fmt.Sprint(key), minVersion, versioned)
}

func tierSingleflightKeyParts(name, key string, minVersion int64, versioned bool) string {
	if versioned {
		return tierSingleflightVersionedKeyParts(name, key, minVersion)
	}
	var versionBuf [20]byte
	version := strconv.AppendInt(versionBuf[:0], minVersion, 10)
	size := len(name) + len(key) + len(version) + 2
	var stack [128]byte
	out := stack[:0]
	if size > len(stack) {
		out = make([]byte, 0, size)
	}
	out = append(out, name...)
	out = append(out, ':')
	out = append(out, key...)
	out = append(out, ':')
	out = append(out, version...)
	return string(out)
}

func tierSingleflightVersionedKeyParts(name, key string, minVersion int64) string {
	const versionedSuffix = ":versioned"

	var versionBuf [20]byte
	version := strconv.AppendInt(versionBuf[:0], minVersion, 10)
	size := len(name) + len(key) + len(version) + 2 + len(versionedSuffix)
	var stack [128]byte
	out := stack[:0]
	if size > len(stack) {
		out = make([]byte, 0, size)
	}
	out = append(out, name...)
	out = append(out, ':')
	out = append(out, key...)
	out = append(out, ':')
	out = append(out, version...)
	out = append(out, versionedSuffix...)
	return string(out)
}
