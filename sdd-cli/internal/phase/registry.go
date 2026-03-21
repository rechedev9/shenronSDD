package phase

import "time"

// builtinPhases defines all 10 SDD pipeline phases in pipeline order.
// Assemble fields are nil here — wired by context.init() via SetAssembler.
var builtinPhases = []Phase{
	{
		Name:          "explore",
		Prerequisites: []string{},
		NextPhases:    []string{"propose"},
		ArtifactFile:  "exploration.md",
		CacheInputs:   []string{},
		CacheTTL:      0,
	},
	{
		Name:          "propose",
		Prerequisites: []string{"explore"},
		NextPhases:    []string{"spec", "design"},
		ArtifactFile:  "proposal.md",
		CacheInputs:   []string{"exploration.md"},
		CacheTTL:      4 * time.Hour,
	},
	{
		Name:          "spec",
		Prerequisites: []string{"propose"},
		NextPhases:    []string{"tasks"},
		ArtifactFile:  "specs",
		CacheInputs:   []string{"proposal.md", "exploration.md"},
		CacheTTL:      2 * time.Hour,
	},
	{
		Name:          "design",
		Prerequisites: []string{"propose"},
		NextPhases:    []string{"tasks"},
		ArtifactFile:  "design.md",
		CacheInputs:   []string{"proposal.md", "specs/"},
		CacheTTL:      2 * time.Hour,
	},
	{
		Name:          "tasks",
		Prerequisites: []string{"spec", "design"},
		NextPhases:    []string{"apply"},
		ArtifactFile:  "tasks.md",
		CacheInputs:   []string{"design.md", "specs/"},
		CacheTTL:      1 * time.Hour,
	},
	{
		Name:          "apply",
		Prerequisites: []string{"tasks"},
		NextPhases:    []string{"review"},
		ArtifactFile:  "tasks.md",
		RecoverSkip:   true, // same artifact as tasks; skip in Recover()
		CacheInputs:   []string{"tasks.md", "design.md", "specs/"},
		CacheTTL:      30 * time.Minute,
	},
	{
		Name:          "review",
		Prerequisites: []string{"apply"},
		NextPhases:    []string{"verify"},
		ArtifactFile:  "review-report.md",
		CacheInputs:   []string{"tasks.md", "design.md", "specs/"},
		CacheTTL:      1 * time.Hour,
	},
	{
		Name:          "verify",
		Prerequisites: []string{"review"},
		NextPhases:    []string{"clean"},
		ArtifactFile:  "verify-report.md",
		CacheInputs:   []string{},
		CacheTTL:      0,
	},
	{
		Name:          "clean",
		Prerequisites: []string{"verify"},
		NextPhases:    []string{"archive"},
		ArtifactFile:  "clean-report.md",
		CacheInputs:   []string{"verify-report.md", "tasks.md", "design.md", "specs/"},
		CacheTTL:      1 * time.Hour,
	},
	{
		Name:          "archive",
		Prerequisites: []string{"clean"},
		NextPhases:    []string{},
		ArtifactFile:  "archive-manifest.md",
		CacheInputs:   []string{},
		CacheTTL:      0,
	},
}

func init() {
	for _, p := range builtinPhases {
		DefaultRegistry.Register(p)
	}
}
