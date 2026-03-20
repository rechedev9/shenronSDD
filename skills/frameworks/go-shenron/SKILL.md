---
name: go-shenron
description: >
  Go patterns from production CLIs.
  Trigger: When writing Go code - CLIs, error handling, testing, project structure.
license: Apache-2.0
metadata:
  version: "1.1"
  source: "production Go CLI patterns"
---

## Thin Shell, Fat Core (REQUIRED)

```go
// ✅ ALWAYS: main.go is 5-15 lines, delegates to internal/cli
// cmd/myapp/main.go
package main

import (
    "context"
    "os"

    "github.com/you/myapp/internal/cli"
)

func main() {
    if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
        os.Exit(cli.ExitCode(err))
    }
}

// ❌ NEVER: Logic in main.go
func main() {
    cfg := loadConfig()       // NO — belongs in internal/
    db := openDB(cfg.DSN)     // NO
    runServer(db)              // NO
}
```

**Why?** main.go is stable infrastructure. All changes happen in the library. DI at the boundary makes the entire CLI testable without mocking globals.

## Project Layout (REQUIRED)

```
cmd/myapp/main.go          # Entry point — 5-15 lines only
internal/
  cli/                      # CLI wiring, command definitions
  config/                   # Config loading
  store/                    # Persistence (SQLite, files)
  <domain>/                 # Domain-specific packages
```

One file per concern within a package: `errors.go`, `types.go`, `client.go`, `db.go`, `migrations.go`.

```go
// ✅ Package-per-context, domain-based
internal/discord/    // Discord API client
internal/syncer/     // Sync orchestration
internal/store/      // SQLite persistence

// ❌ NEVER: Flat layout or pkg/
pkg/utils/           // NO — Go uses internal/
internal/helpers.go  // NO — split by domain
```

**Why?** Predictable structure — an LLM navigating any repo can predict where code lives. `internal/` enforces encapsulation at the compiler level.

## Consumer-Defined Interfaces (REQUIRED)

```go
// ✅ ALWAYS: Define the interface where it's consumed, not where it's implemented
// internal/app/app.go — the CONSUMER defines what it needs
type WAClient interface {
    Close()
    IsAuthed() bool
    Connect(ctx context.Context, opts ConnectOptions) error
    SendText(ctx context.Context, to JID, text string) (MessageID, error)
}

type App struct {
    client WAClient  // Accepts any implementation
}

// ❌ NEVER: Define interfaces at the provider
// internal/whatsapp/client.go
type Client interface {  // NO — 30 methods the consumer doesn't need
    // ...massive surface area
}
```

**Why?** Consumer-defined interfaces are naturally minimal and testable. The LLM sees exactly what the consumer needs, not a sprawling API surface.

## Error Handling — fmt.Errorf with %w (REQUIRED)

```go
// ✅ ALWAYS: Wrap errors with context at every call site
func (s *Store) Open(path string) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return fmt.Errorf("create db directory: %w", err)
    }
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return fmt.Errorf("open sqlite: %w", err)
    }
    if err := db.Ping(); err != nil {
        return fmt.Errorf("ping sqlite: %w", err)
    }
    s.db = db
    return nil
}

// ✅ Custom error types ONLY where classification matters
type HTTPError struct {
    StatusCode int
    Body       string
}

func (e *HTTPError) Error() string {
    return fmt.Sprintf("http %d: %s", e.StatusCode, e.Body)
}

// ✅ Sentinel errors only for package-level conditions
var ErrNoOrigin = errors.New("no origin configured")

// ❌ NEVER: Naked error returns without context
if err != nil {
    return err  // NO — loses call site information
}

// ❌ NEVER: Excessive custom types for simple wrapping
type DatabaseError struct{ Err error }  // NO — fmt.Errorf is enough
```

**Why?** `%w` wrapping builds a stack trace via error context. `errors.Is`/`errors.As` unwrap the chain. Custom types are only justified when callers need to branch on error kind.

## CLI Framework — Cobra or Kong

```go
// ✅ Cobra for simpler CLIs
var rootCmd = &cobra.Command{
    Use:   "myapp",
    Short: "One-line description",
}

// ✅ Kong for complex subcommand trees
type CLI struct {
    Fetch FetchCmd `cmd:"" help:"Fetch data"`
    Sync  SyncCmd  `cmd:"" help:"Sync to local"`
}

// ✅ Default subcommand injection pattern (from sag)
func maybeDefaultToSpeak() {
    first := os.Args[1]
    if isKnownSubcommand(first) || isCobraBuiltin(first) {
        return
    }
    os.Args = append([]string{os.Args[0], "speak"}, os.Args[1:]...)
}

// ❌ NEVER: Hand-rolling arg parsing for non-trivial CLIs
// ❌ NEVER: Heavy HTTP frameworks (gin, echo) for CLI tools
```

## Version Injection via ldflags (REQUIRED)

```go
// internal/cli/version.go
var version = "dev"  // Overwritten at build time

// Makefile or .goreleaser.yaml
// ldflags: -s -w -X github.com/you/myapp/internal/cli.version={{ .Version }}
```

## Dependencies — Standard Library First (REQUIRED)

```go
// ✅ Minimal dependency trees
// Common in production Go CLIs: cobra, kong, go-toml, modernc.org/sqlite

// ✅ *http.Client configured directly with explicit timeouts
client := &http.Client{
    Timeout: 30 * time.Second,
}

// ❌ NEVER: ORM (gorm, ent)
// ❌ NEVER: Heavy HTTP frameworks (gin, echo, fiber) for CLI tools
// ❌ NEVER: DI containers (wire, dig, fx)
// ❌ NEVER: Mock generation frameworks (gomock, mockgen)
```

**Why?** Each dependency is replaceable and total surface area stays small. The Go stdlib covers HTTP, JSON, testing, and concurrency without third-party weight.

## SQLite — WAL Mode with Standard Pragmas

```go
// ✅ ALWAYS: Configure SQLite with these pragmas
pragmas := []string{
    "PRAGMA journal_mode=WAL",
    "PRAGMA synchronous=NORMAL",
    "PRAGMA temp_store=MEMORY",
    "PRAGMA foreign_keys=ON",
    "PRAGMA busy_timeout=5000",
}

// ✅ Inline migrations, no framework
func (s *Store) migrate(ctx context.Context) error {
    stmts := []string{
        `CREATE TABLE IF NOT EXISTS guilds (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL
        )`,
        `CREATE TABLE IF NOT EXISTS channels (
            id TEXT PRIMARY KEY,
            guild_id TEXT NOT NULL REFERENCES guilds(id)
        )`,
    }
    for _, stmt := range stmts {
        if _, err := s.db.ExecContext(ctx, stmt); err != nil {
            return fmt.Errorf("migrate: %w", err)
        }
    }
    return nil
}

// ✅ Pure Go driver for CGO_ENABLED=0
import _ "modernc.org/sqlite"

// ✅ CGO driver ONLY when performance-critical
import _ "github.com/mattn/go-sqlite3"
```

## Testing — Table-Driven, Hand-Written Fakes (REQUIRED)

```go
// ✅ ALWAYS: Table-driven tests
func TestInferFormat(t *testing.T) {
    tests := []struct {
        path string
        want string
    }{
        {"out.mp3", "mp3_44100_128"},
        {"audio.wav", "pcm_44100"},
        {"video.mp4", ""},
    }
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            got := inferFormatFromExt(tt.path)
            if got != tt.want {
                t.Errorf("inferFormatFromExt(%q) = %q, want %q", tt.path, got, tt.want)
            }
        })
    }
}

// ✅ Hand-written fakes implementing consumer-defined interfaces
type fakeWA struct {
    mu        sync.Mutex
    authed    bool
    connected bool
}

func newFakeWA() *fakeWA { return &fakeWA{authed: true} }

func (f *fakeWA) IsAuthed() bool {
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.authed
}

// ✅ In-memory SQLite with t.TempDir() for DB tests
func TestStore(t *testing.T) {
    db := newTestDB(t, filepath.Join(t.TempDir(), "test.db"))
    // ...
}

// ✅ t.Parallel() where appropriate
// ✅ testify's require for assertions (optional)

// ❌ NEVER: gomock / mockgen
// ❌ NEVER: Global test fixtures — each test owns its setup
```

## Build & Release (REQUIRED)

```makefile
# ✅ Consistent Makefile targets
.PHONY: fmt lint test check build

fmt:
	gofumpt -w .

lint:
	golangci-lint run

test:
	go test ./...

check: fmt lint test

build:
	CGO_ENABLED=0 go build -o bin/myapp ./cmd/myapp
```

```yaml
# ✅ .goreleaser.yaml for cross-platform releases
builds:
  - env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags: -s -w -X github.com/you/myapp/internal/cli.version={{ .Version }}
```

```yaml
# ✅ .golangci.yml for linting
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
```

## Keywords
go, golang, cli, cobra, kong, sqlite, error handling, testing, table-driven, interfaces, rsn
