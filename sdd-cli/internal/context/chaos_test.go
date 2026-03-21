package context

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// TestChaosConcurrentAssemble assembles context for different phases concurrently.
// Tests that the global phase registry + assembler wiring is safe.
func TestChaosConcurrentAssemble(t *testing.T) {
	t.Parallel()

	// Create a minimal project structure per goroutine.
	phases := []state.Phase{
		state.PhaseExplore, state.PhasePropose,
	}

	var wg sync.WaitGroup
	wg.Add(len(phases) * 5)

	for _, ph := range phases {
		for i := 0; i < 5; i++ {
			go func(p state.Phase) {
				defer wg.Done()
				dir := t.TempDir()
				changeDir := filepath.Join(dir, "openspec", "changes", "chaos")
				os.MkdirAll(changeDir, 0o755)

				st := state.NewState("chaos", "chaos test")
				state.Save(st, filepath.Join(changeDir, "state.json"))

				// Write minimal config.
				cfgDir := filepath.Join(dir, "openspec")
				os.MkdirAll(cfgDir, 0o755)
				os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nstack:\n  language: go\n  build_tool: go\ncommands:\n  build: echo ok\n  lint: echo ok\n  test: echo ok\n"), 0o644)

				cfg, _ := config.Load(filepath.Join(cfgDir, "config.yaml"))
				if cfg == nil {
					return
				}

				var buf bytes.Buffer
				params := &Params{
					ChangeDir:  changeDir,
					ChangeName: "chaos",
					ProjectDir: dir,
					Config:     cfg,
					SkillsPath: filepath.Join(dir, "skills"),
				}
				// Assembly may fail (missing SKILL.md) — that's fine.
				// We're testing for races, not correctness.
				_ = Assemble(&buf, p, params)
			}(ph)
		}
	}

	wg.Wait()
}

// TestChaosConcurrentExtract runs all extract functions concurrently on the same content.
func TestChaosConcurrentExtract(t *testing.T) {
	t.Parallel()

	content := `# Design
## Decisions
- Use WebSocket for real-time updates
- Use ECharts for visualization

## Architecture
Bottom-up approach

# Tasks
- [x] Create hub.go
- [ ] Write tests
- [x] Update server.go
`
	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = extractDecisions(content)
				_ = extractFirst(content, "## Architecture", 10)
				_ = extractCompletedTasks(content)
				_ = extractCurrentTask(content)
			}
		}()
	}

	wg.Wait()
}
