package cdc

import (
	"strconv"
	"time"
)

func marshalTerminalRunPayload(eventType, runID, jobID, projectID, status string, attempt int, errMessage string, timestamp time.Time) []byte {
	var attemptBuf [20]byte
	attemptBytes := strconv.AppendInt(attemptBuf[:0], int64(attempt), 10)
	var timestampBuf [len("2006-01-02T15:04:05.999999999Z07:00")]byte
	timestampBytes := timestamp.AppendFormat(timestampBuf[:0], time.RFC3339Nano)
	out := make([]byte, 0, len(`{"event_type":"","run_id":"","job_id":"","project_id":"","status":"","attempt":,"error":"","timestamp":""}`)+len(eventType)+len(runID)+len(jobID)+len(projectID)+len(status)+len(attemptBytes)+len(errMessage)+len(timestampBytes))
	out = append(out, `{"event_type":`...)
	out = strconv.AppendQuote(out, eventType)
	out = append(out, `,"run_id":`...)
	out = strconv.AppendQuote(out, runID)
	out = append(out, `,"job_id":`...)
	out = strconv.AppendQuote(out, jobID)
	out = append(out, `,"project_id":`...)
	out = strconv.AppendQuote(out, projectID)
	out = append(out, `,"status":`...)
	out = strconv.AppendQuote(out, status)
	out = append(out, `,"attempt":`...)
	out = append(out, attemptBytes...)
	out = append(out, `,"error":`...)
	out = strconv.AppendQuote(out, errMessage)
	out = append(out, `,"timestamp":`...)
	out = appendJSONTimeBytes(out, timestampBytes)
	out = append(out, '}')
	return out
}

func appendJSONTimeBytes(out []byte, timestamp []byte) []byte {
	out = append(out, '"')
	out = append(out, timestamp...)
	out = append(out, '"')
	return out
}
