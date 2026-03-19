package ci

import (
	"bytes"
	"fmt"
	"text/template"
)

// GenerateConfig holds template values for CI config generation.
type GenerateConfig struct {
	ProjectID   string
	Environment string
}

// Generate creates a CI workflow file for the given provider.
func Generate(provider string, cfg GenerateConfig) (string, error) {
	tplStr, ok := templates[provider]
	if !ok {
		return "", fmt.Errorf("unsupported CI provider: %s", provider)
	}

	tpl, err := template.New(provider).Parse(tplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

var templates = map[string]string{
	"github": `name: Strait Deploy

on:
  push:
    branches: [main, master]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Strait CLI
        run: |
          curl -fsSL https://get.strait.run | sh
          echo "$HOME/.strait/bin" >> $GITHUB_PATH

      - name: Build manifest
        run: strait build

      - name: Validate
        run: strait check -f .strait/manifest.json

      - name: Deploy
        run: strait deploy --config strait.json --env {{.Environment}}
        env:
          STRAIT_API_KEY: ${{"{{"}} secrets.STRAIT_API_KEY {{"}}"}}
          STRAIT_PROJECT: {{.ProjectID}}
`,
	"gitlab": `stages:
  - validate
  - deploy

validate:
  stage: validate
  script:
    - curl -fsSL https://get.strait.run | sh
    - export PATH="$HOME/.strait/bin:$PATH"
    - strait build
    - strait check -f .strait/manifest.json

deploy:
  stage: deploy
  script:
    - curl -fsSL https://get.strait.run | sh
    - export PATH="$HOME/.strait/bin:$PATH"
    - strait deploy --config strait.json --env {{.Environment}}
  only:
    - main
    - master
  variables:
    STRAIT_API_KEY: $STRAIT_API_KEY
    STRAIT_PROJECT: {{.ProjectID}}
`,
	"generic": `#!/bin/bash
set -euo pipefail

# Install Strait CLI
curl -fsSL https://get.strait.run | sh
export PATH="$HOME/.strait/bin:$PATH"

# Build and validate
strait build
strait check -f .strait/manifest.json

# Deploy
export STRAIT_API_KEY="${STRAIT_API_KEY}"
export STRAIT_PROJECT="{{.ProjectID}}"
strait deploy --config strait.json --env {{.Environment}}
`,
}
