package notification

import (
	"encoding/json"
	"strings"
)

func marshalSlackPayload(eventType string, payload []byte) ([]byte, error) {
	return json.Marshal(slackPayload{Text: chatPayloadText(eventType, payload)})
}

func marshalDiscordPayload(eventType string, payload []byte) ([]byte, error) {
	return json.Marshal(discordPayload{Content: chatPayloadText(eventType, payload)})
}

type slackPayload struct {
	Text string `json:"text"`
}

type discordPayload struct {
	Content string `json:"content"`
}

func chatPayloadText(eventType string, payload []byte) string {
	var text strings.Builder
	text.Grow(len(eventType) + len(payload) + len("[] "))
	text.WriteByte('[')
	text.WriteString(eventType)
	text.WriteString("] ")
	text.Write(payload)
	return text.String()
}
