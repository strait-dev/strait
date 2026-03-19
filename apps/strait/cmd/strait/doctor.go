package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Check   string `json:"check"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

func newDoctorCommand(state *appState) *cobra.Command {
	var verbose bool
	var asJSON bool
	var fix bool
	var checkEndpoints bool
	var checkManifests string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run comprehensive system health checks",
		Long: `Runs parallel checks across CLI version, configuration, server connectivity,
authentication, and environment variables to diagnose common issues.`,
		Example: `  strait doctor
  strait doctor --verbose
  strait doctor --json
  strait doctor --check-endpoints
  strait doctor --check-manifests ./manifests/`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var mu sync.Mutex
			checks := make([]doctorCheck, 0, 16)

			addCheck := func(c doctorCheck) {
				mu.Lock()
				defer mu.Unlock()
				checks = append(checks, c)
			}

			p := pool.New().WithMaxGoroutines(8)

			// 1. CLI version check
			p.Go(func() {
				detail := fmt.Sprintf("version=%s commit=%s go=%s os=%s/%s", version, commit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
				if version == "dev" {
					addCheck(doctorCheck{
						Check:   "cli_version",
						Status:  "warn",
						Message: "running dev build",
						Fix:     "install a released version or build with -ldflags",
					})
				} else {
					msg := detail
					if !verbose {
						msg = version
					}
					addCheck(doctorCheck{
						Check:   "cli_version",
						Status:  "pass",
						Message: msg,
					})
				}
			})

			// 2. Config validity
			p.Go(func() {
				if state.configPath == "" {
					addCheck(doctorCheck{
						Check:   "config",
						Status:  "warn",
						Message: "no config file loaded",
						Fix:     "run `strait init` to create a config file",
					})
					return
				}
				addCheck(doctorCheck{
					Check:   "config",
					Status:  "pass",
					Message: state.configPath,
				})
			})

			// 3. Server URL configured
			p.Go(func() {
				if state.opts.serverURL == "" {
					addCheck(doctorCheck{
						Check:   "server_url",
						Status:  "fail",
						Message: "not configured",
						Fix:     "set --server flag or STRAIT_SERVER env var",
					})
					return
				}
				addCheck(doctorCheck{
					Check:   "server_url",
					Status:  "pass",
					Message: state.opts.serverURL,
				})
			})

			// 4. API key present
			p.Go(func() {
				if state.opts.apiKey == "" {
					addCheck(doctorCheck{
						Check:   "api_key",
						Status:  "fail",
						Message: boolString(false),
						Fix:     "run `strait login` or set STRAIT_API_KEY",
					})
					return
				}
				addCheck(doctorCheck{
					Check:   "api_key",
					Status:  "pass",
					Message: boolString(true),
				})
			})

			// 5. Project ID set
			p.Go(func() {
				if state.opts.projectID == "" {
					addCheck(doctorCheck{
						Check:   "project_id",
						Status:  "warn",
						Message: "not set",
						Fix:     "set --project flag or STRAIT_PROJECT env var",
					})
					return
				}
				addCheck(doctorCheck{
					Check:   "project_id",
					Status:  "pass",
					Message: state.opts.projectID,
				})
			})

			// 6. Connectivity: health endpoint
			p.Go(func() {
				cli, err := newAPIClient(state)
				if err != nil {
					addCheck(doctorCheck{
						Check:   "connectivity_health",
						Status:  "fail",
						Message: err.Error(),
						Fix:     "check --server URL and network connectivity",
					})
					return
				}
				health, hErr := cli.Health(cmd.Context())
				addCheck(doctorCheck{
					Check:   "connectivity_health",
					Status:  statusFromErr(hErr),
					Message: healthDetail(health, hErr),
					Fix:     fixIfErr(hErr, "verify server is running and reachable"),
				})
			})

			// 7. Connectivity: ready endpoint
			p.Go(func() {
				cli, err := newAPIClient(state)
				if err != nil {
					addCheck(doctorCheck{
						Check:   "connectivity_ready",
						Status:  "fail",
						Message: err.Error(),
						Fix:     "check --server URL and network connectivity",
					})
					return
				}
				ready, rErr := cli.HealthReady(cmd.Context())
				addCheck(doctorCheck{
					Check:   "connectivity_ready",
					Status:  statusFromErr(rErr),
					Message: healthDetail(ready, rErr),
					Fix:     fixIfErr(rErr, "verify database and redis dependencies are up"),
				})
			})

			// 8. Auth: stats endpoint
			p.Go(func() {
				cli, err := newAPIClient(state)
				if err != nil {
					addCheck(doctorCheck{
						Check:   "auth_stats",
						Status:  "fail",
						Message: err.Error(),
						Fix:     "check API client configuration",
					})
					return
				}
				stats, sErr := cli.Stats(cmd.Context())
				addCheck(doctorCheck{
					Check:   "auth_stats",
					Status:  statusFromErr(sErr),
					Message: statsDetail(stats, sErr),
					Fix:     fixIfErr(sErr, "check server auth and /v1/stats availability"),
				})
			})

			// 9. TCP connectivity
			p.Go(func() {
				if state.opts.serverURL == "" {
					return
				}
				host, port, splitErr := splitHostPortFromURL(state.opts.serverURL)
				if splitErr != nil {
					addCheck(doctorCheck{
						Check:   "tcp_connectivity",
						Status:  "fail",
						Message: splitErr.Error(),
						Fix:     "check server URL format",
					})
					return
				}
				target := net.JoinHostPort(host, port)
				dialer := net.Dialer{Timeout: 2 * time.Second}
				conn, dialErr := dialer.DialContext(cmd.Context(), "tcp", target)
				if dialErr != nil {
					addCheck(doctorCheck{
						Check:   "tcp_connectivity",
						Status:  "fail",
						Message: dialErr.Error(),
						Fix:     "check DNS/network and server port reachability",
					})
					return
				}
				_ = conn.Close()
				addCheck(doctorCheck{
					Check:   "tcp_connectivity",
					Status:  "pass",
					Message: target,
				})
			})

			// 10. Environment variables
			p.Go(func() {
				envVars := []struct {
					name string
					fix  string
				}{
					{"DATABASE_URL", "export DATABASE_URL=..."},
					{"REDIS_URL", "export REDIS_URL=..."},
					{"INTERNAL_SECRET", "export INTERNAL_SECRET=..."},
					{"JWT_SIGNING_KEY", "export JWT_SIGNING_KEY=..."},
				}
				for _, ev := range envVars {
					val := strings.TrimSpace(os.Getenv(ev.name))
					if val == "" {
						addCheck(doctorCheck{
							Check:   "env_" + strings.ToLower(ev.name),
							Status:  "warn",
							Message: "not set",
							Fix:     ev.fix,
						})
					} else if verbose {
						addCheck(doctorCheck{
							Check:   "env_" + strings.ToLower(ev.name),
							Status:  "pass",
							Message: "set",
						})
					}
				}
			})

			// 11. Optional: check manifest files
			if checkManifests != "" {
				p.Go(func() {
					manifests, err := loadManifestInputs([]string{checkManifests})
					if err != nil {
						addCheck(doctorCheck{
							Check:   "manifests",
							Status:  "fail",
							Message: err.Error(),
							Fix:     "check manifest path",
						})
						return
					}
					if len(manifests) == 0 {
						addCheck(doctorCheck{
							Check:   "manifests",
							Status:  "warn",
							Message: "no manifests found at path",
							Fix:     "verify the manifest directory contains YAML files",
						})
						return
					}
					valid := 0
					for _, item := range manifests {
						if validateManifest(item.Data) == nil {
							valid++
						}
					}
					addCheck(doctorCheck{
						Check:   "manifests",
						Status:  statusFromBool(valid == len(manifests)),
						Message: fmt.Sprintf("%d/%d valid", valid, len(manifests)),
						Fix:     fixIfBool(valid != len(manifests), "run `strait check -f "+checkManifests+"` for details"),
					})
				})
			}

			// 12. Optional: check endpoints from manifests
			if checkEndpoints && checkManifests != "" {
				p.Go(func() {
					manifests, err := loadManifestInputs([]string{checkManifests})
					if err != nil {
						return
					}
					for _, item := range manifests {
						if strings.ToLower(item.Data.Kind) != "job" {
							continue
						}
						endpointURL := getString(item.Data.Spec, "endpoint_url")
						if endpointURL == "" {
							continue
						}
						if reachErr := checkEndpointReachable(endpointURL, 3*time.Second); reachErr != nil {
							addCheck(doctorCheck{
								Check:   "endpoint:" + item.Data.Metadata.Name,
								Status:  "fail",
								Message: reachErr.Error(),
								Fix:     "verify endpoint URL is correct and reachable",
							})
						} else {
							addCheck(doctorCheck{
								Check:   "endpoint:" + item.Data.Metadata.Name,
								Status:  "pass",
								Message: endpointURL,
							})
						}
					}
				})
			}

			p.Wait()

			// Auto-fix: only config initialization for now
			if fix {
				if state.configPath == "" {
					// Suggest fix but don't auto-run it since that would modify the filesystem
					for i, c := range checks {
						if c.Check == "config" && c.Status != "pass" {
							checks[i].Message += " (use `strait init` to fix)"
						}
					}
				}
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(checks)
			}

			// Table output
			rows := make([]map[string]any, 0, len(checks))
			for _, c := range checks {
				row := map[string]any{
					"check":   c.Check,
					"status":  c.Status,
					"message": c.Message,
				}
				if c.Fix != "" {
					row["fix"] = c.Fix
				}
				rows = append(rows, row)
			}

			if err := printData(state, rows); err != nil {
				return err
			}

			// Summary line
			passed, warned, failed := 0, 0, 0
			for _, c := range checks {
				switch c.Status {
				case "pass":
					passed++
				case "warn":
					warned++
				case "fail":
					failed++
				}
			}
			fmt.Fprintf(os.Stderr, "\n%d passed, %d warnings, %d failed\n", passed, warned, failed)

			if failed > 0 {
				return fmt.Errorf("doctor found %d failing check(s)", failed)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "show detailed check output")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output results as JSON")
	cmd.Flags().BoolVar(&fix, "fix", false, "attempt to auto-fix issues where possible")
	cmd.Flags().BoolVar(&checkEndpoints, "check-endpoints", false, "test endpoint URL reachability from manifests")
	cmd.Flags().StringVar(&checkManifests, "check-manifests", "", "path to manifest files or directory to validate")

	return cmd
}

func statusFromErr(err error) string {
	if err == nil {
		return "pass"
	}
	return "fail"
}

func statusFromBool(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func fixIfErr(err error, fix string) string {
	if err != nil {
		return fix
	}
	return ""
}

func fixIfBool(failed bool, fix string) string {
	if failed {
		return fix
	}
	return ""
}
