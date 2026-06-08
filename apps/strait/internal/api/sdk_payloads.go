package api

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"strait/internal/domain"
)

func apiRunPubSubChannel(runID string) string {
	return "run:" + runID
}

type sdkResourceSampleData struct {
	MemoryMB      float64 `json:"memory_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	CPUPercent    float64 `json:"cpu_percent"`
}

type sdkOOMRiskData struct {
	MemoryMB      float64 `json:"memory_mb"`
	MemoryLimitMB float64 `json:"memory_limit_mb"`
	UsagePercent  float64 `json:"usage_percent"`
}

type sdkRunEventPayload struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	RunID     string          `json:"run_id"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

type sdkProgressData struct {
	Percent    float64 `json:"percent"`
	Step       string  `json:"step,omitempty"`
	ETASeconds int     `json:"eta_seconds,omitempty"`
}

type sdkProgressEventPayload struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	RunID     string          `json:"run_id"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Data      sdkProgressData `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

func marshalSDKStreamChunkPayload(chunk, streamID string, done bool, timestamp time.Time) ([]byte, error) {
	out := make([]byte, 0, 104+len(chunk)+len(streamID))
	out = append(out, `{"type":"stream_chunk","chunk":`...)
	out = strconv.AppendQuote(out, chunk)
	out = append(out, `,"stream_id":`...)
	out = strconv.AppendQuote(out, streamID)
	out = append(out, `,"done":`...)
	out = strconv.AppendBool(out, done)
	out = append(out, `,"timestamp":`...)
	out = appendSDKJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}

func marshalSDKResourceSampleData(memoryMB, memoryPercent, cpuPercent float64) ([]byte, error) {
	payload := sdkResourceSampleData{
		MemoryMB:      memoryMB,
		MemoryPercent: memoryPercent,
		CPUPercent:    cpuPercent,
	}
	if !isFiniteJSONFloat(memoryMB) || !isFiniteJSONFloat(memoryPercent) || !isFiniteJSONFloat(cpuPercent) {
		return json.Marshal(payload)
	}

	out := make([]byte, 0, 80)
	out = append(out, `{"memory_mb":`...)
	out = strconv.AppendFloat(out, memoryMB, 'f', -1, 64)
	out = append(out, `,"memory_percent":`...)
	out = strconv.AppendFloat(out, memoryPercent, 'f', -1, 64)
	out = append(out, `,"cpu_percent":`...)
	out = strconv.AppendFloat(out, cpuPercent, 'f', -1, 64)
	out = append(out, '}')
	return out, nil
}

func marshalSDKOOMRiskData(memoryMB, memoryLimitMB float64) ([]byte, error) {
	usagePercent := memoryMB / memoryLimitMB * 100
	payload := sdkOOMRiskData{
		MemoryMB:      memoryMB,
		MemoryLimitMB: memoryLimitMB,
		UsagePercent:  usagePercent,
	}
	if !isFiniteJSONFloat(memoryMB) ||
		!isFiniteJSONFloat(memoryLimitMB) ||
		!isFiniteJSONFloat(usagePercent) {
		return json.Marshal(payload)
	}

	out := make([]byte, 0, 88)
	out = append(out, `{"memory_mb":`...)
	out = strconv.AppendFloat(out, memoryMB, 'f', -1, 64)
	out = append(out, `,"memory_limit_mb":`...)
	out = strconv.AppendFloat(out, memoryLimitMB, 'f', -1, 64)
	out = append(out, `,"usage_percent":`...)
	out = strconv.AppendFloat(out, usagePercent, 'f', -1, 64)
	out = append(out, '}')
	return out, nil
}

func marshalSDKStatusChangePayload(runID string, from, to string, timestamp time.Time) ([]byte, error) {
	out := make([]byte, 0, 104+len(runID)+len(from)+len(to))
	out = append(out, `{"type":"status_change","run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"from":`...)
	out = strconv.AppendQuote(out, from)
	out = append(out, `,"to":`...)
	out = strconv.AppendQuote(out, to)
	out = append(out, `,"timestamp":`...)
	out = appendSDKJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}

func marshalSDKFailedStatusChangePayload(runID string, from, to, errMessage string, timestamp time.Time) ([]byte, error) {
	out := make([]byte, 0, 112+len(runID)+len(from)+len(to)+len(errMessage))
	out = append(out, `{"type":"status_change","run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"from":`...)
	out = strconv.AppendQuote(out, from)
	out = append(out, `,"to":`...)
	out = strconv.AppendQuote(out, to)
	out = append(out, `,"error":`...)
	out = strconv.AppendQuote(out, errMessage)
	out = append(out, `,"timestamp":`...)
	out = appendSDKJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}

func marshalSDKRunEventPayload(
	eventType domain.EventType,
	runID, level, message string,
	data json.RawMessage,
	timestamp time.Time,
) ([]byte, error) {
	payload := sdkRunEventPayload{
		Type:      "event",
		EventType: string(eventType),
		RunID:     runID,
		Level:     level,
		Message:   message,
		Data:      data,
		Timestamp: timestamp,
	}
	if data != nil && !json.Valid(data) {
		return json.Marshal(payload)
	}

	out := make([]byte, 0, 128+len(eventType)+len(runID)+len(level)+len(message)+len(data))
	out = append(out, `{"type":"event","event_type":`...)
	out = strconv.AppendQuote(out, string(eventType))
	out = append(out, `,"run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"level":`...)
	out = strconv.AppendQuote(out, level)
	out = append(out, `,"message":`...)
	out = strconv.AppendQuote(out, message)
	out = append(out, `,"data":`...)
	if data == nil {
		out = append(out, "null"...)
	} else {
		out = append(out, data...)
	}
	out = append(out, `,"timestamp":`...)
	out = appendSDKJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}

func marshalSDKProgressData(percent float64, step string, etaSeconds int) ([]byte, error) {
	payload := sdkProgressData{
		Percent:    percent,
		Step:       step,
		ETASeconds: etaSeconds,
	}
	if !isFiniteJSONFloat(percent) {
		return json.Marshal(payload)
	}

	out := make([]byte, 0, 64+len(step))
	out = append(out, `{"percent":`...)
	out = strconv.AppendFloat(out, percent, 'f', -1, 64)
	if step != "" {
		out = append(out, `,"step":`...)
		out = strconv.AppendQuote(out, step)
	}
	if etaSeconds != 0 {
		out = append(out, `,"eta_seconds":`...)
		out = strconv.AppendInt(out, int64(etaSeconds), 10)
	}
	out = append(out, '}')
	return out, nil
}

func isFiniteJSONFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func appendSDKJSONTime(out []byte, timestamp time.Time) []byte {
	out = append(out, '"')
	out = timestamp.AppendFormat(out, time.RFC3339Nano)
	out = append(out, '"')
	return out
}

func marshalSDKProgressEventPayload(
	runID, message string,
	percent float64,
	step string,
	etaSeconds int,
	timestamp time.Time,
) ([]byte, error) {
	payload := sdkProgressEventPayload{
		Type:      "event",
		EventType: string(domain.EventProgress),
		RunID:     runID,
		Level:     "info",
		Message:   message,
		Data: sdkProgressData{
			Percent:    percent,
			Step:       step,
			ETASeconds: etaSeconds,
		},
		Timestamp: timestamp,
	}
	if !isFiniteJSONFloat(percent) {
		return json.Marshal(payload)
	}

	out := make([]byte, 0, 168+len(runID)+len(message)+len(step))
	out = append(out, `{"type":"event","event_type":"progress","run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"level":"info","message":`...)
	out = strconv.AppendQuote(out, message)
	out = append(out, `,"data":{"percent":`...)
	out = strconv.AppendFloat(out, percent, 'f', -1, 64)
	if step != "" {
		out = append(out, `,"step":`...)
		out = strconv.AppendQuote(out, step)
	}
	if etaSeconds != 0 {
		out = append(out, `,"eta_seconds":`...)
		out = strconv.AppendInt(out, int64(etaSeconds), 10)
	}
	out = append(out, `},"timestamp":`...)
	out = appendSDKJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}
