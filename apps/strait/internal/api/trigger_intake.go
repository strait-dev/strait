package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"sort"
	"time"
	"unicode/utf8"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidwall/gjson"
)

func mergedRunTags(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	if len(base) == 0 {
		return maps.Clone(overlay)
	}
	if len(overlay) == 0 {
		return maps.Clone(base)
	}
	runTags := make(map[string]string, len(base)+len(overlay))
	maps.Copy(runTags, base)
	maps.Copy(runTags, overlay)
	return runTags
}

func mergeRunMetadata(metadata, defaults map[string]string) map[string]string {
	if len(metadata) == 0 && len(defaults) == 0 {
		return nil
	}
	if len(metadata) == 0 {
		return maps.Clone(defaults)
	}
	if len(defaults) == 0 {
		return maps.Clone(metadata)
	}
	merged := make(map[string]string, len(defaults)+len(metadata))
	maps.Copy(merged, metadata)
	for key, value := range defaults {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	return merged
}

func applyDefaultRunMetadata(metadata, defaults map[string]string) map[string]string {
	if len(defaults) == 0 {
		if len(metadata) == 0 {
			return nil
		}
		return metadata
	}
	if len(metadata) == 0 {
		return maps.Clone(defaults)
	}
	for key, value := range defaults {
		if _, exists := metadata[key]; !exists {
			metadata[key] = value
		}
	}
	return metadata
}

func ensureJobTriggerable(job *domain.Job) error {
	if !job.Enabled {
		return huma.Error400BadRequest("job is disabled")
	}
	if job.Paused {
		return huma.Error409Conflict("job is paused -- resume it before triggering new runs")
	}
	return nil
}

func (s *Server) validateTriggerJobInput(input *TriggerJobInput, req *TriggerRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return newValidationError(err)
	}
	if err := validateTriggerTraceHeaders(input); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validatePayloadSize(req.Payload); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTags(req.Tags); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	if err := validateTriggerScheduledAt(req.ScheduledAt); err != nil {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

func validateTriggerScheduledAt(scheduledAt *time.Time) error {
	if scheduledAt == nil {
		return nil
	}
	delay := time.Until(*scheduledAt)
	if delay < 0 {
		return errors.New("scheduled_at must not be in the past")
	}
	if delay > 30*24*time.Hour {
		return errors.New("scheduled_at cannot exceed 30 days from now")
	}
	return nil
}

func validateTriggerTTLSecs(ttlSecs *int) error {
	if ttlSecs == nil {
		return nil
	}
	if *ttlSecs < 0 {
		return errors.New("ttl_secs must be greater than or equal to 0")
	}
	if *ttlSecs > maxTriggerTTLSecs {
		return errors.New("ttl_secs cannot exceed 30 days")
	}
	return nil
}

const canonicalEmptyPayloadHash = "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

func canonicalizePayload(payload json.RawMessage) (json.RawMessage, string, error) {
	if len(payload) == 0 || (len(payload) == 2 && payload[0] == '{' && payload[1] == '}') {
		return json.RawMessage(`{}`), canonicalEmptyPayloadHash, nil
	}
	if !gjson.ValidBytes(payload) {
		return canonicalizePayloadSlow(payload)
	}

	canonical, err := appendCanonicalJSON(make([]byte, 0, len(payload)), gjson.ParseBytes(payload))
	if err != nil {
		return canonicalizePayloadSlow(payload)
	}

	hash := sha256.Sum256(canonical)
	var hashHex [sha256.Size * 2]byte
	hex.Encode(hashHex[:], hash[:])
	return canonical, string(hashHex[:]), nil
}

func canonicalizePayloadSlow(payload json.RawMessage) (json.RawMessage, string, error) {
	var v any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return nil, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", errors.New("payload must contain a single JSON value")
	}

	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(canonical)
	var hashHex [sha256.Size * 2]byte
	hex.Encode(hashHex[:], hash[:])
	return canonical, string(hashHex[:]), nil
}

var errDuplicateCanonicalObjectKey = errors.New("duplicate object key")

type canonicalJSONMember struct {
	key   string
	value gjson.Result
}

func appendCanonicalJSON(dst []byte, value gjson.Result) ([]byte, error) {
	switch value.Type {
	case gjson.Null:
		return append(dst, "null"...), nil
	case gjson.False:
		return append(dst, "false"...), nil
	case gjson.True:
		return append(dst, "true"...), nil
	case gjson.Number:
		return append(dst, value.Raw...), nil
	case gjson.String:
		return appendCanonicalJSONString(dst, value.Str), nil
	case gjson.JSON:
		if value.IsObject() {
			return appendCanonicalJSONObject(dst, value)
		}
		if value.IsArray() {
			return appendCanonicalJSONArray(dst, value)
		}
	}
	return nil, errors.New("unsupported JSON value")
}

func appendCanonicalJSONObject(dst []byte, object gjson.Result) ([]byte, error) {
	members := make([]canonicalJSONMember, 0, 2)
	object.ForEach(func(key, value gjson.Result) bool {
		members = append(members, canonicalJSONMember{key: key.Str, value: value})
		return true
	})
	sort.Slice(members, func(i, j int) bool {
		return members[i].key < members[j].key
	})

	dst = append(dst, '{')
	for i, member := range members {
		if i > 0 {
			if member.key == members[i-1].key {
				return nil, errDuplicateCanonicalObjectKey
			}
			dst = append(dst, ',')
		}
		dst = appendCanonicalJSONString(dst, member.key)
		dst = append(dst, ':')
		next, err := appendCanonicalJSON(dst, member.value)
		if err != nil {
			return nil, err
		}
		dst = next
	}
	dst = append(dst, '}')
	return dst, nil
}

func appendCanonicalJSONArray(dst []byte, array gjson.Result) ([]byte, error) {
	dst = append(dst, '[')
	first := true
	var appendErr error
	array.ForEach(func(_, value gjson.Result) bool {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst, appendErr = appendCanonicalJSON(dst, value)
		return appendErr == nil
	})
	if appendErr != nil {
		return nil, appendErr
	}
	dst = append(dst, ']')
	return dst, nil
}

func appendCanonicalJSONString(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == '\\' || c == '"' || c == '<' || c == '>' || c == '&' || c >= utf8.RuneSelf {
			quoted, _ := json.Marshal(s)
			return append(dst, quoted...)
		}
	}
	dst = append(dst, '"')
	dst = append(dst, s...)
	dst = append(dst, '"')
	return dst
}
