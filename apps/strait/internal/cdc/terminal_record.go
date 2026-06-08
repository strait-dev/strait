package cdc

import (
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
)

type terminalRunRecord struct {
	ID        string
	JobID     string
	ProjectID string
	Status    string
	Attempt   int
	Error     string
}

func parseTerminalRunRecord(record []byte) (terminalRunRecord, error) {
	if !gjson.ValidBytes(record) {
		return terminalRunRecord{}, fmt.Errorf("invalid JSON")
	}

	id, err := optionalJSONString(record, "id")
	if err != nil {
		return terminalRunRecord{}, err
	}
	jobID, err := optionalJSONString(record, "job_id")
	if err != nil {
		return terminalRunRecord{}, err
	}
	projectID, err := optionalJSONString(record, "project_id")
	if err != nil {
		return terminalRunRecord{}, err
	}
	status, err := optionalJSONString(record, "status")
	if err != nil {
		return terminalRunRecord{}, err
	}
	errMessage, err := optionalJSONString(record, "error")
	if err != nil {
		return terminalRunRecord{}, err
	}
	attempt, err := optionalJSONInt(record, "attempt")
	if err != nil {
		return terminalRunRecord{}, err
	}

	return terminalRunRecord{
		ID:        id,
		JobID:     jobID,
		ProjectID: projectID,
		Status:    status,
		Attempt:   attempt,
		Error:     errMessage,
	}, nil
}

func optionalJSONString(record []byte, field string) (string, error) {
	value := gjson.GetBytes(record, field)
	if !value.Exists() || value.Type == gjson.Null {
		return "", nil
	}
	if value.Type != gjson.String {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value.String(), nil
}

func optionalJSONInt(record []byte, field string) (int, error) {
	value := gjson.GetBytes(record, field)
	if !value.Exists() || value.Type == gjson.Null {
		return 0, nil
	}
	if value.Type != gjson.Number {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	parsed, err := strconv.ParseInt(value.Raw, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return int(parsed), nil
}
