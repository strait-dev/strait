package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

func newCheckCommand(state *appState) *cobra.Command {
	var files []string
	var checkEndpoints bool
	var endpointTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate configuration files with deep checks",
		Long: `Runs comprehensive validation on declarative definition files.

Validates YAML syntax, required fields, cron expressions, workflow DAG
acyclicity, and optionally checks endpoint URL reachability.`,
		Example: `  strait check -f jobs.yaml
  strait check -f jobs.yaml -f workflows.yaml --check-endpoints
  strait check -f manifests/ --endpoint-timeout 5s`,
		RunE: func(_ *cobra.Command, _ []string) error {
			manifests, err := loadManifestInputs(files)
			if err != nil {
				return err
			}
			if len(manifests) == 0 {
				return fmt.Errorf("no manifests found")
			}

			results := make([]map[string]any, 0, len(manifests))

			for _, item := range manifests {
				checks := make([]map[string]any, 0, 4)

				// 1. Basic validation (kind, metadata, spec)
				if err := validateManifest(item.Data); err != nil {
					checks = append(checks, checkResult("syntax", false, err.Error()))
					results = append(results, manifestResult(item, checks))
					continue
				}
				checks = append(checks, checkResult("syntax", true, ""))

				kind := strings.ToLower(item.Data.Kind)

				// 2. Cron expression validation (jobs only)
				if kind == "job" {
					cronExpr := getString(item.Data.Spec, "cron")
					if cronExpr != "" {
						if _, cronErr := cron.ParseStandard(cronExpr); cronErr != nil {
							checks = append(checks, checkResult("cron", false, fmt.Sprintf("invalid cron expression %q: %v", cronExpr, cronErr)))
						} else {
							checks = append(checks, checkResult("cron", true, fmt.Sprintf("valid: %s", cronExpr)))
						}
					}
				}

				// 3. DAG acyclicity check (workflows only)
				if kind == "workflow" {
					if stepsRaw, ok := item.Data.Spec["steps"]; ok {
						if dagErr := checkDAG(stepsRaw); dagErr != nil {
							checks = append(checks, checkResult("dag", false, dagErr.Error()))
						} else {
							checks = append(checks, checkResult("dag", true, "acyclic"))
						}
					}
				}

				// 4. Endpoint reachability (jobs only, optional)
				if kind == "job" && checkEndpoints {
					endpointURL := getString(item.Data.Spec, "endpoint_url")
					if endpointURL != "" {
						if reachErr := checkEndpointReachable(endpointURL, endpointTimeout); reachErr != nil {
							checks = append(checks, checkResult("endpoint", false, reachErr.Error()))
						} else {
							checks = append(checks, checkResult("endpoint", true, endpointURL))
						}
					}
				}

				results = append(results, manifestResult(item, checks))
			}

			if err := printData(state, results); err != nil {
				return err
			}

			for _, r := range results {
				if checks, ok := r["checks"].([]map[string]any); ok {
					for _, check := range checks {
						if ok, _ := check["ok"].(bool); !ok {
							return fmt.Errorf("check failed")
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "manifest file, directory, or - for stdin")
	cmd.Flags().BoolVar(&checkEndpoints, "check-endpoints", false, "test endpoint URL reachability")
	cmd.Flags().DurationVar(&endpointTimeout, "endpoint-timeout", 3*time.Second, "timeout for endpoint reachability checks")

	return cmd
}

func checkResult(name string, ok bool, detail string) map[string]any {
	return map[string]any{
		"check":  name,
		"ok":     ok,
		"detail": detail,
	}
}

func manifestResult(item loadedManifest, checks []map[string]any) map[string]any {
	allOK := true
	for _, c := range checks {
		if ok, _ := c["ok"].(bool); !ok {
			allOK = false
			break
		}
	}
	return map[string]any{
		"source": item.Source,
		"kind":   item.Data.Kind,
		"name":   item.Data.Metadata.Name,
		"valid":  allOK,
		"checks": checks,
	}
}

// checkDAG validates that workflow steps form a valid DAG (no cycles).
func checkDAG(stepsRaw any) error {
	steps, ok := stepsRaw.([]any)
	if !ok {
		return fmt.Errorf("steps must be an array")
	}

	refs := make(map[string]bool)
	deps := make(map[string][]string)

	for _, stepRaw := range steps {
		step, ok := stepRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("step must be an object")
		}
		ref := getString(step, "step_ref")
		if ref == "" {
			return fmt.Errorf("step missing step_ref")
		}
		if refs[ref] {
			return fmt.Errorf("duplicate step_ref %q", ref)
		}
		refs[ref] = true

		if depsList, ok := step["depends_on"].([]any); ok {
			for _, d := range depsList {
				if ds, ok := d.(string); ok {
					deps[ref] = append(deps[ref], ds)
				}
			}
		}
	}

	// Validate all dependencies reference existing steps
	for ref, parents := range deps {
		for _, p := range parents {
			if !refs[p] {
				return fmt.Errorf("step %q depends on unknown step %q", ref, p)
			}
		}
	}

	// Kahn's algorithm for topological sort / cycle detection
	inDegree := make(map[string]int)
	for ref := range refs {
		inDegree[ref] = 0
	}
	for ref, parents := range deps {
		inDegree[ref] = len(parents)
	}

	queue := make([]string, 0)
	for ref, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, ref)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++

		// Find all steps that depend on this node
		for ref, parents := range deps {
			for _, p := range parents {
				if p == node {
					inDegree[ref]--
					if inDegree[ref] == 0 {
						queue = append(queue, ref)
					}
				}
			}
		}
	}

	if visited != len(refs) {
		return fmt.Errorf("cycle detected in step dependencies")
	}

	return nil
}

// checkEndpointReachable does a HEAD request to verify the endpoint is reachable.
func checkEndpointReachable(endpoint string, timeout time.Duration) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check DNS resolution first
	host := parsed.Hostname()
	if _, err := net.LookupHost(host); err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	resp, err := client.Head(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint unreachable: %w", err)
	}
	_ = resp.Body.Close()

	return nil
}
