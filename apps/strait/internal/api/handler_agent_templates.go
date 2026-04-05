package api

import (
	"context"
	"encoding/json"
)

type AgentTemplate struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Model       string          `json:"model"`
	Config      json.RawMessage `json:"config"`
}

type ListAgentTemplatesOutput struct {
	Body []AgentTemplate
}

var builtinAgentTemplates = []AgentTemplate{
	{
		Name:        "Incident Triage",
		Slug:        "incident-triage",
		Description: "Classifies incoming incidents by severity and routes to the correct on-call team.",
		Category:    "operations",
		Model:       "gpt-4o-mini",
		Config:      json.RawMessage(`{"temperature":0.1,"max_attempts":3,"timeout_secs":120}`),
	},
	{
		Name:        "Content Classifier",
		Slug:        "content-classifier",
		Description: "Classifies content into categories with confidence scores.",
		Category:    "content",
		Model:       "gpt-4o-mini",
		Config:      json.RawMessage(`{"temperature":0,"timeout_secs":60}`),
	},
	{
		Name:        "Code Reviewer",
		Slug:        "code-reviewer",
		Description: "Reviews code changes for bugs, security issues, and style violations.",
		Category:    "engineering",
		Model:       "claude-sonnet-4-5",
		Config:      json.RawMessage(`{"temperature":0.2,"timeout_secs":180}`),
	},
	{
		Name:        "Data Extractor",
		Slug:        "data-extractor",
		Description: "Extracts structured data from unstructured text, PDFs, and web pages.",
		Category:    "content",
		Model:       "gpt-4o",
		Config:      json.RawMessage(`{"temperature":0,"timeout_secs":120}`),
	},
	{
		Name:        "Support Router",
		Slug:        "support-router",
		Description: "Routes support tickets to the correct team based on content analysis.",
		Category:    "operations",
		Model:       "gpt-4o-mini",
		Config:      json.RawMessage(`{"temperature":0.1,"timeout_secs":60}`),
	},
}

func (s *Server) handleListAgentTemplates(_ context.Context, _ *struct{}) (*ListAgentTemplatesOutput, error) {
	return &ListAgentTemplatesOutput{Body: builtinAgentTemplates}, nil
}
