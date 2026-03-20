package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	localConfigName = ".strait.yaml"
	serviceDirName  = "strait"
	configFileName  = "config.yaml"
)

type Context struct {
	Server  string `yaml:"server,omitempty"`
	Project string `yaml:"project,omitempty"`
	Format  string `yaml:"format,omitempty"`
}

type File struct {
	ServerURL      string              `yaml:"server,omitempty"`
	Token          string              `yaml:"api_key,omitempty"`
	DefaultProject string              `yaml:"project,omitempty"`
	OutputFormat   string              `yaml:"format,omitempty"`
	Aliases        map[string]string   `yaml:"aliases,omitempty"`
	Secrets        map[string][]string `yaml:"secrets,omitempty"`
	ActiveContext  string              `yaml:"active_context,omitempty"`
	Contexts       map[string]Context  `yaml:"contexts,omitempty"`
}

type LoadResult struct {
	Path    string
	Exists  bool
	IsLocal bool
	Data    *File
}

type ResolveInput struct {
	Flags           map[string]string
	BoolFlags       map[string]bool
	DurationFlags   map[string]string
	Changed         map[string]bool
	Config          *File
	Env             map[string]string
	ContextOverride string
}

type Resolved struct {
	ServerURL   string
	Credential  string
	ProjectID   string
	Format      string
	ContextName string
	NoColor     bool
	Quiet       bool
	Verbose     bool
	Timeout     string
	ConfigPath  string
}

func Load(pathOverride string) (*LoadResult, error) {
	paths, err := candidatePaths(pathOverride)
	if err != nil {
		return nil, err
	}

	var localPath string
	if pathOverride == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr == nil {
			localPath = filepath.Join(cwd, localConfigName)
		}
	}

	for _, p := range paths {
		st, statErr := os.Stat(p)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat config %q: %w", p, statErr)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("config path %q is a directory", p)
		}

		content, readErr := os.ReadFile(p) //nolint:gosec // config path is explicit override or trusted discovery path
		if readErr != nil {
			return nil, fmt.Errorf("read config %q: %w", p, readErr)
		}

		cfg := &File{}
		if len(content) > 0 {
			if unmarshalErr := yaml.Unmarshal(content, cfg); unmarshalErr != nil {
				return nil, fmt.Errorf("parse config %q: %w", p, unmarshalErr)
			}
		}
		normalize(cfg)

		isLocal := false
		if localPath != "" {
			resolvedP, _ := filepath.EvalSymlinks(p)
			resolvedLocal, _ := filepath.EvalSymlinks(localPath)
			if resolvedP == resolvedLocal {
				isLocal = true
			}
		}

		return &LoadResult{Path: p, Exists: true, IsLocal: isLocal, Data: cfg}, nil
	}

	defaultPath := paths[len(paths)-1]
	return &LoadResult{Path: defaultPath, Exists: false, Data: newDefault()}, nil
}

func Save(path string, cfg *File) error {
	normalize(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if writeErr := os.WriteFile(path, encoded, 0o600); writeErr != nil {
		return fmt.Errorf("write config file: %w", writeErr)
	}

	return nil
}

func Resolve(input ResolveInput) Resolved {
	cfg := input.Config
	if cfg == nil {
		cfg = newDefault()
	}
	normalize(cfg)

	active := cfg.ActiveContext
	if envContext := strings.TrimSpace(input.Env["STRAIT_CONTEXT"]); envContext != "" {
		active = envContext
	}
	if input.ContextOverride != "" {
		active = input.ContextOverride
	}

	resolved := Resolved{
		ServerURL:   strings.TrimSpace(input.Env["STRAIT_SERVER"]),
		Credential:  strings.TrimSpace(input.Env["STRAIT_API_KEY"]),
		ProjectID:   strings.TrimSpace(input.Env["STRAIT_PROJECT"]),
		Format:      strings.TrimSpace(input.Env["STRAIT_FORMAT"]),
		ContextName: active,
		NoColor:     input.BoolFlags["no-color"] || strings.TrimSpace(input.Env["NO_COLOR"]) != "",
		Quiet:       input.BoolFlags["quiet"],
		Verbose:     input.BoolFlags["verbose"],
		Timeout:     strings.TrimSpace(input.DurationFlags["timeout"]),
	}

	if cfg.ServerURL != "" {
		resolved.ServerURL = cfg.ServerURL
	}
	if cfg.Token != "" {
		resolved.Credential = cfg.Token
	}
	if cfg.DefaultProject != "" {
		resolved.ProjectID = cfg.DefaultProject
	}
	if cfg.OutputFormat != "" {
		resolved.Format = cfg.OutputFormat
	}

	if ctx, ok := cfg.Contexts[active]; ok {
		if ctx.Server != "" {
			resolved.ServerURL = ctx.Server
		}
		if ctx.Project != "" {
			resolved.ProjectID = ctx.Project
		}
		if ctx.Format != "" {
			resolved.Format = ctx.Format
		}
	}

	if input.Changed["server"] {
		resolved.ServerURL = strings.TrimSpace(input.Flags["server"])
	}
	if input.Changed["api-key"] {
		resolved.Credential = strings.TrimSpace(input.Flags["api-key"])
	}
	if input.Changed["project"] {
		resolved.ProjectID = strings.TrimSpace(input.Flags["project"])
	}
	if input.Changed["format"] {
		resolved.Format = strings.TrimSpace(input.Flags["format"])
	}
	if input.Changed["context"] {
		resolved.ContextName = strings.TrimSpace(input.Flags["context"])
	}
	if input.Changed["no-color"] {
		resolved.NoColor = input.BoolFlags["no-color"]
	}
	if input.Changed["quiet"] {
		resolved.Quiet = input.BoolFlags["quiet"]
	}
	if input.Changed["verbose"] {
		resolved.Verbose = input.BoolFlags["verbose"]
	}
	if input.Changed["timeout"] {
		resolved.Timeout = input.DurationFlags["timeout"]
	}

	if resolved.ServerURL == "" {
		resolved.ServerURL = "http://localhost:8080"
	}
	if resolved.Format == "" {
		resolved.Format = "table"
	}
	if resolved.Timeout == "" {
		resolved.Timeout = "30s"
	}

	return resolved
}

func candidatePaths(pathOverride string) ([]string, error) {
	if pathOverride != "" {
		return []string{pathOverride}, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get user home: %w", err)
	}

	return []string{
		filepath.Join(cwd, localConfigName),
		filepath.Join(home, ".config", serviceDirName, configFileName),
	}, nil
}

func normalize(cfg *File) {
	if cfg == nil {
		return
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	if cfg.Secrets == nil {
		cfg.Secrets = map[string][]string{}
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]Context{}
	}
}

func newDefault() *File {
	return &File{
		Aliases:  map[string]string{},
		Secrets:  map[string][]string{},
		Contexts: map[string]Context{},
	}
}

// HomePath returns the path to the user's home config file only.
func HomePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home: %w", err)
	}
	return filepath.Join(home, ".config", serviceDirName, configFileName), nil
}

// HasSensitiveLocalFields returns field names that are set in cfg among
// security-sensitive fields: server, api_key, active_context, contexts, aliases.
func HasSensitiveLocalFields(cfg *File) []string {
	if cfg == nil {
		return nil
	}
	var fields []string
	if cfg.ServerURL != "" {
		fields = append(fields, "server")
	}
	if cfg.Token != "" {
		fields = append(fields, "api_key")
	}
	if cfg.ActiveContext != "" {
		fields = append(fields, "active_context")
	}
	if len(cfg.Contexts) > 0 {
		fields = append(fields, "contexts")
	}
	if len(cfg.Aliases) > 0 {
		fields = append(fields, "aliases")
	}
	return fields
}
