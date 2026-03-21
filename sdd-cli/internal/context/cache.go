package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"
)

// cacheVersion is bumped when assembler output format changes.
// Any cache entry written with a different version is treated as stale.
// Bump this when: adding new sections to assemblers, changing section
// labels, modifying summary format, or changing what artifacts are loaded.
// Bumped to 6 for phase-interface refactor (cache input lookup changed).
const cacheVersion = 6

// Phase TTL values are now in the phase registry.
// See internal/phase/registry.go for the canonical definitions.

// cacheDir returns the cache directory for a change.
func cacheDir(changeDir string) string {
	return filepath.Join(changeDir, ".cache")
}

// contextCachePath returns the path to the cached context for a phase.
func contextCachePath(changeDir, phase string) string {
	return filepath.Join(cacheDir(changeDir), phase+".ctx")
}

// hashCachePath returns the path to the hash file for a phase.
func hashCachePath(changeDir, phase string) string {
	return filepath.Join(cacheDir(changeDir), phase+".hash")
}

// Phase cache inputs are now in the phase registry.
// See internal/phase/registry.go for the canonical definitions.

// phaseCacheInputs returns CacheInputs for a phase from the registry.
func phaseCacheInputs(name string) []string {
	desc, ok := phase.DefaultRegistry.Get(name)
	if !ok {
		return nil
	}
	return desc.CacheInputs
}

// phaseCacheTTL returns CacheTTL for a phase from the registry.
func phaseCacheTTL(name string) time.Duration {
	desc, ok := phase.DefaultRegistry.Get(name)
	if !ok {
		return 0
	}
	return desc.CacheTTL
}

// inputHash computes a SHA256 hash of all input artifacts + SKILL.md for a phase.
// Includes cacheVersion so format changes auto-invalidate.
// Includes SKILL.md so skill edits invalidate the cache (tokentally ETag pattern).
func inputHash(changeDir string, inputs []string, skillsPath, phaseName string) string {
	h := sha256.New()

	// Version prefix.
	fmt.Fprintf(h, "v%d:", cacheVersion)

	// Hash the SKILL.md for this phase — fixes correctness bug where
	// editing a skill wouldn't invalidate cached context.
	if skillsPath != "" && phaseName != "" {
		skillPath := filepath.Join(skillsPath, "sdd-"+phaseName, "SKILL.md")
		if data, err := os.ReadFile(skillPath); err == nil {
			fmt.Fprintf(h, "skill:%d:", len(data))
			h.Write(data)
		}
	}

	sorted := make([]string, len(inputs))
	copy(sorted, inputs)
	sort.Strings(sorted)

	for _, name := range sorted {
		if name == "specs/" {
			hashSpecsDir(h, changeDir)
			continue
		}
		data, err := os.ReadFile(filepath.Join(changeDir, name))
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s:%d:", name, len(data))
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// hashSpecsDir hashes all .md files in specs/ into the provided hasher.
func hashSpecsDir(h io.Writer, changeDir string) {
	specsDir := filepath.Join(changeDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specsDir, e.Name()))
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "specs/%s:%d:", e.Name(), len(data))
		h.Write(data)
	}
}

// tryCachedContext checks if a cached context exists, its input hash
// matches the current artifacts, and the TTL hasn't expired.
// Hash file format: "{hex_hash}|{unix_seconds}"
// Legacy files without "|" produce a cache miss (silent upgrade).
func tryCachedContext(changeDir, phaseName, skillsPath string) ([]byte, bool) {
	inputs := phaseCacheInputs(phaseName)

	raw, err := os.ReadFile(hashCachePath(changeDir, phaseName))
	if err != nil {
		return nil, false
	}

	// Parse "hash|timestamp" format.
	stored := strings.TrimSpace(string(raw))
	storedHash, tsStr, ok := strings.Cut(stored, "|")
	if !ok {
		return nil, false // legacy format without timestamp → miss
	}

	// Check content hash (includes SKILL.md).
	currentHash := inputHash(changeDir, inputs, skillsPath, phaseName)
	if storedHash != currentHash {
		return nil, false
	}

	// Check TTL.
	if ttl := phaseCacheTTL(phaseName); ttl > 0 {
		ts := mustParseInt64(tsStr)
		age := time.Since(time.Unix(ts, 0))
		if age > ttl {
			return nil, false // expired
		}
	}

	cached, err := os.ReadFile(contextCachePath(changeDir, phaseName))
	if err != nil {
		return nil, false
	}

	return cached, true
}

func mustParseInt64(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0 // epoch → forces TTL miss (safe fallback)
	}
	return v
}

// saveContextCache stores the assembled context and its input hash with timestamp.
// Format: "{hash}|{unix_seconds}"
func saveContextCache(changeDir, phaseName, skillsPath string, content []byte) error {
	dir := cacheDir(changeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	inputs := phaseCacheInputs(phaseName)
	hash := inputHash(changeDir, inputs, skillsPath, phaseName)
	hashWithTS := fmt.Sprintf("%s|%d", hash, time.Now().Unix())

	hashPath := hashCachePath(changeDir, phaseName)
	tmp := hashPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(hashWithTS), 0o644); err != nil {
		return fmt.Errorf("write hash cache: %w", err)
	}
	if err := os.Rename(tmp, hashPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename hash cache: %w", err)
	}

	ctxPath := contextCachePath(changeDir, phaseName)
	tmp = ctxPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("write context cache: %w", err)
	}
	if err := os.Rename(tmp, ctxPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename context cache: %w", err)
	}

	return nil
}

// estimateTokens provides a rough token estimate from byte count.
// ~4 bytes per token for English/code mixed content.
func estimateTokens(size int) int {
	return size / 4
}

// maxContextBytes is the default size limit for assembled context.
// ~100KB ≈ 25K tokens — keeps sub-agents within context window.
const maxContextBytes = 100 * 1024

// contextMetrics holds measurements from a context assembly operation.
type contextMetrics struct {
	Phase      string
	Bytes      int
	Tokens     int
	Cached     bool
	DurationMs int64
}

// writeMetrics prints context metrics to stderr.
// and appends to the cumulative metrics log for the change.
func writeMetrics(w io.Writer, m *contextMetrics, verbosity int) {
	if verbosity < 0 {
		return
	}
	source := "assembled"
	if m.Cached {
		source = "cached"
	}
	// Oracle-style: ↑context Δtokens
	fmt.Fprintf(w, "sdd: phase=%s ↑%s Δ%dK tokens %dms (%s)\n",
		m.Phase,
		formatBytes(m.Bytes),
		m.Tokens/1000,
		m.DurationMs,
		source,
	)
}

// PipelineMetrics tracks cumulative token usage across all phases of a change.
// Exported for use by sdd health command.
type PipelineMetrics struct {
	Version     int                     `json:"version"`
	Phases      map[string]PhaseMetrics `json:"phases"`
	TotalBytes  int                     `json:"total_bytes"`
	TotalTokens int                     `json:"total_tokens"`
	CacheHits   int                     `json:"cache_hits"`
	CacheMisses int                     `json:"cache_misses"`
}

// PhaseMetrics holds per-phase metrics. Exported for sdd health.
type PhaseMetrics struct {
	Bytes      int   `json:"bytes"`
	Tokens     int   `json:"tokens"`
	Cached     bool  `json:"cached"`
	DurationMs int64 `json:"duration_ms"`
}

// metricsPath returns the path to the cumulative metrics file.
func metricsPath(changeDir string) string {
	return filepath.Join(cacheDir(changeDir), "metrics.json")
}

// recordMetrics appends a phase's metrics to the cumulative tracker.
// Best-effort — failures are silently ignored.
func recordMetrics(changeDir string, m *contextMetrics) {
	pm := LoadPipelineMetrics(changeDir)

	pm.Phases[m.Phase] = PhaseMetrics{
		Bytes:      m.Bytes,
		Tokens:     m.Tokens,
		Cached:     m.Cached,
		DurationMs: m.DurationMs,
	}

	// Recompute totals.
	pm.TotalBytes = 0
	pm.TotalTokens = 0
	pm.CacheHits = 0
	pm.CacheMisses = 0
	for _, p := range pm.Phases {
		pm.TotalBytes += p.Bytes
		pm.TotalTokens += p.Tokens
		if p.Cached {
			pm.CacheHits++
		} else {
			pm.CacheMisses++
		}
	}

	data, err := json.MarshalIndent(pm, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(cacheDir(changeDir), 0o755)
	mp := metricsPath(changeDir)
	tmp := mp + ".tmp"
	if os.WriteFile(tmp, data, 0o644) != nil {
		return
	}
	if os.Rename(tmp, mp) != nil {
		os.Remove(tmp)
	}
}

// LoadPipelineMetrics reads the cumulative metrics file for a change.
// Exported for use by sdd health command.
func LoadPipelineMetrics(changeDir string) *PipelineMetrics {
	data, err := os.ReadFile(metricsPath(changeDir))
	if err != nil {
		return &PipelineMetrics{
			Version: cacheVersion,
			Phases:  make(map[string]PhaseMetrics),
		}
	}

	var pm PipelineMetrics
	if err := json.Unmarshal(data, &pm); err != nil || pm.Version != cacheVersion {
		return &PipelineMetrics{
			Version: cacheVersion,
			Phases:  make(map[string]PhaseMetrics),
		}
	}

	return &pm
}

func formatBytes(b int) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	return fmt.Sprintf("%dKB", b/1024)
}

// CheckCacheIntegrity counts stale cache entries in a change directory.
// Returns the number of .hash files whose stored hash no longer matches
// the current input hash (content drift).
func CheckCacheIntegrity(changeDir, skillsPath string) (int, error) {
	stale := 0
	hashFiles, err := filepath.Glob(filepath.Join(cacheDir(changeDir), "*.hash"))
	if err != nil || len(hashFiles) == 0 {
		return 0, nil
	}
	for _, hf := range hashFiles {
		phase := strings.TrimSuffix(filepath.Base(hf), ".hash")
		raw, err := os.ReadFile(hf)
		if err != nil {
			continue
		}
		stored := strings.TrimSpace(string(raw))
		storedHash, _, ok := strings.Cut(stored, "|")
		if !ok {
			stale++
			continue
		}
		inputs := phaseCacheInputs(phase)
		current := inputHash(changeDir, inputs, skillsPath, phase)
		if storedHash != current {
			stale++
		}
	}
	return stale, nil
}
