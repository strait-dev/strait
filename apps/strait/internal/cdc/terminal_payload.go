package cdc

import (
	"strconv"
	"time"
)

func marshalTerminalRunPayload(eventType, runID, jobID, projectID, status string, attempt int, errMessage string, timestamp time.Time) []byte {
	out := make([]byte, 0, len(eventType)+len(runID)+len(jobID)+len(projectID)+len(status)+len(errMessage)+160)
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
	out = strconv.AppendInt(out, int64(attempt), 10)
	out = append(out, `,"error":`...)
	out = strconv.AppendQuote(out, errMessage)
	out = append(out, `,"timestamp":`...)
	out = appendJSONTime(out, timestamp)
	out = append(out, '}')
	return out
}

func appendJSONTime(out []byte, timestamp time.Time) []byte {
	out = append(out, '"')
	out = timestamp.AppendFormat(out, time.RFC3339Nano)
	out = append(out, '"')
	return out
}
