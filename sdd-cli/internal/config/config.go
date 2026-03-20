package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var ErrNoManifest = errors.New("no recognized project manifest found")

// manifestInfo maps manifest filenames to language/stack detection info.
type manifestInfo struct {
	Language  string
	BuildTool string
	BuildCmd  string
	TestCmd   string
	LintCmd   string
	FormatCmd string
}

// Ordered so the first match wins in monorepo scenarios.
var manifests = []struct {
	File string
	Info manifestInfo
}{
	{"go.mod", manifestInfo{
		Language: "go", BuildTool: "go",
		BuildCmd: "go build ./...", TestCmd: "go test ./...",
		LintCmd: "golangci-lint run ./...", FormatCmd: "gofumpt -w .",
	}},
	{"package.json", manifestInfo{
		Language: "typescript", BuildTool: "npm",
		BuildCmd: "npm run build", TestCmd: "npm test",
		LintCmd: "npm run lint", FormatCmd: "npm run format",
	}},
	{"pyproject.toml", manifestInfo{
		Language: "python", BuildTool: "pip",
		BuildCmd: "", TestCmd: "pytest",
		LintCmd: "ruff check .", FormatCmd: "ruff format .",
	}},
	{"Cargo.toml", manifestInfo{
		Language: "rust", BuildTool: "cargo",
		BuildCmd: "cargo build", TestCmd: "cargo test",
		LintCmd: "cargo clippy", FormatCmd: "cargo fmt",
	}},
	{"build.gradle", manifestInfo{
		Language: "java", BuildTool: "gradle",
		BuildCmd: "./gradlew build", TestCmd: "./gradlew test",
		LintCmd: "", FormatCmd: "",
	}},
	{"pom.xml", manifestInfo{
		Language: "java", BuildTool: "maven",
		BuildCmd: "mvn compile", TestCmd: "mvn test",
		LintCmd: "", FormatCmd: "",
	}},
}

// Detect scans projectDir for known manifest files and returns a Config.
func Detect(projectDir string) (*Config, error) {
	var found []string
	var primary *manifestInfo

	for _, m := range manifests {
		path := filepath.Join(projectDir, m.File)
		if _, err := os.Stat(path); err == nil {
			found = append(found, m.File)
			if primary == nil {
				info := m.Info
				primary = &info
			}
		}
	}

	if primary == nil {
		return nil, fmt.Errorf("%w: scanned %s", ErrNoManifest, projectDir)
	}

	name := filepath.Base(projectDir)

	cfg := &Config{
		ProjectName: name,
		Stack: Stack{
			Language:  primary.Language,
			BuildTool: primary.BuildTool,
			Manifests: found,
		},
		Commands: Commands{
			Build:  primary.BuildCmd,
			Test:   primary.TestCmd,
			Lint:   primary.LintCmd,
			Format: primary.FormatCmd,
		},
		SkillsPath: defaultSkillsPath(),
	}

	return cfg, nil
}

// Load reads a config.yaml file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes a Config to path as YAML.
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}

func defaultSkillsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.claude/skills/sdd/"
	}
	return filepath.Join(home, ".claude", "skills", "sdd")
}
