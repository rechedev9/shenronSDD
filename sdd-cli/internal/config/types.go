package config

// Config represents the project-level SDD configuration (openspec/config.yaml).
type Config struct {
	Version       int            `yaml:"version"        json:"version"`
	ProjectName   string         `yaml:"project_name"   json:"project_name"`
	Stack         Stack          `yaml:"stack"           json:"stack"`
	SkillsPath    string         `yaml:"skills_path"     json:"skills_path"`
	Commands      Commands       `yaml:"commands"        json:"commands"`
	Capabilities  Capabilities   `yaml:"capabilities"    json:"capabilities"`
}

// Stack describes the detected tech stack.
type Stack struct {
	Language   string   `yaml:"language"    json:"language"`
	Framework  string   `yaml:"framework"   json:"framework"`
	BuildTool  string   `yaml:"build_tool"  json:"build_tool"`
	TestCmd    string   `yaml:"test_cmd"    json:"test_cmd"`
	LintCmd    string   `yaml:"lint_cmd"    json:"lint_cmd"`
	FormatCmd  string   `yaml:"format_cmd"  json:"format_cmd"`
	Manifests  []string `yaml:"manifests"   json:"manifests"`
}

// Commands holds the build/test/lint commands inferred from the project.
type Commands struct {
	Build  string `yaml:"build"  json:"build"`
	Test   string `yaml:"test"   json:"test"`
	Lint   string `yaml:"lint"   json:"lint"`
	Format string `yaml:"format" json:"format"`
}

// Capabilities toggles optional features.
type Capabilities struct {
	MemoryEnabled bool `yaml:"memory_enabled" json:"memory_enabled"`
}
