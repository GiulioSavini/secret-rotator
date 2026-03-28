# Phase 1: Foundation - Research

**Researched:** 2026-03-27
**Domain:** Go CLI scaffold, YAML config loading, .env file handling, Docker container lifecycle
**Confidence:** HIGH

## Summary

Phase 1 builds the foundation layer that every subsequent phase depends on: a CLI skeleton with four subcommands, a YAML configuration loader, an .env file reader/writer with atomic writes, and a Docker Manager for container lifecycle operations. All components are pure Go with no CGO dependencies, and all can be fully unit tested without external services (Docker tests use a mocked interface).

The critical research finding is that Cobra is at **v1.10.2** (not v2 as some sources suggest -- v2 does not exist as a released module). The architecture research correctly identified koanf over Viper, and this is confirmed: Viper's forced key lowercasing breaks env var names like `MYSQL_PASSWORD`. For .env files, no Go library preserves comments and formatting on round-trip, so the tool MUST implement line-level text editing rather than parse-and-reserialize. For Docker, the official `docker/docker/client` SDK provides all needed operations, and `compose-spec/compose-go/v2` can parse compose files to extract `depends_on` relationships for restart ordering.

**Primary recommendation:** Build four packages in order: `internal/cli` (Cobra commands), `internal/config` (koanf loader + validation), `internal/envfile` (line-level reader/writer with atomic writes), `internal/docker` (narrow DockerManager interface wrapping the SDK). Define interfaces first, implement second.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DISC-03 | Tool works zero-config without `rotator.yml`, scanning the current directory | Config loader must treat config file as optional; when absent, use sensible defaults (scan `.` for `.env` files). koanf supports this via optional file loading. |
| DISC-04 | Tool supports multi-file `.env` setups (`.env`, `.env.local`, `docker-compose.override.yml`) | envfile reader must accept multiple paths; config schema `env_files` field supports arrays. Line-level reader handles each file independently. |
| DIST-03 | Tool uses YAML configuration file (`rotator.yml`) for secret definitions | koanf v2 with YAML parser. Config struct with validation. Schema documented in `rotator.example.yml`. |
| INFR-01 | Tool restarts affected containers in dependency order with readiness checks | DockerManager wraps SDK for stop/start/restart/health-check. compose-spec/compose-go parses `depends_on` for ordering. Health polling via `ContainerInspect` checking `Health.Status`. |
| ROT-03 | Tool writes `.env` files atomically (temp file + rename) without corruption | envfile writer uses `os.CreateTemp` in same directory + `f.Sync()` + `os.Rename()`. Preserves permissions via `os.Stat` before write. |
</phase_requirements>

## Standard Stack

### Core (Phase 1 only)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.23+ | Language runtime | Project constraint; single binary, strong stdlib |
| spf13/cobra | v1.10.2 | CLI framework | De-facto Go CLI standard (kubectl, gh, hugo). NOTE: v2 does not exist -- latest stable is v1.10.2 (Dec 2024) |
| knadh/koanf/v2 | v2.3.0 | Configuration loading | Lighter than Viper, no forced key lowercasing (critical for env var names), modular dependencies |
| knadh/koanf/parsers/yaml | latest | YAML parser for koanf | Koanf's YAML backend |
| knadh/koanf/providers/file | latest | File provider for koanf | Reads config from filesystem |
| knadh/koanf/providers/env | latest | Env var provider for koanf | Merges env var overrides into config |
| docker/docker/client | v27.x | Docker SDK | Official Moby client; ContainerList, ContainerInspect, ContainerStop, ContainerStart, ContainerRestart |
| compose-spec/compose-go/v2 | latest | Compose file parser | Parse `depends_on` from docker-compose.yml for restart ordering |
| stretchr/testify | v1.9.x | Test assertions and mocks | Reduces test boilerplate; mock generation for interfaces |
| log/slog | stdlib | Structured logging | Zero dependencies, sufficient for CLI |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| koanf | Viper | Viper lowercases all keys -- breaks `MYSQL_PASSWORD`. Viper pulls HCL, etcd, consul deps. |
| cobra v1.10 | urfave/cli v2 | Less ecosystem adoption, weaker subcommand ergonomics |
| compose-spec/compose-go | Manual YAML parsing | compose-go handles the full compose spec including variable interpolation and extends |
| docker/docker/client | docker/go-sdk | go-sdk too new (Jul 2025), less battle-tested for fine-grained control |
| Custom envfile parser | joho/godotenv | godotenv self-describes as "pretty stupidly naive", does not preserve comments on round-trip |

**Installation (Phase 1 dependencies only):**
```bash
go mod init github.com/giulio/secret-rotator

# CLI
go get github.com/spf13/cobra@v1.10.2

# Config
go get github.com/knadh/koanf/v2@v2.3.0
go get github.com/knadh/koanf/parsers/yaml@latest
go get github.com/knadh/koanf/providers/file@latest
go get github.com/knadh/koanf/providers/env@latest

# Docker
go get github.com/docker/docker@v27
go get github.com/compose-spec/compose-go/v2@latest

# Testing
go get github.com/stretchr/testify@latest
```

## Architecture Patterns

### Recommended Project Structure (Phase 1 scope)

```
secret-rotator/
  cmd/
    rotator/
      main.go              # Entry point -- wire dependencies, call cli.Execute()
  internal/
    cli/                   # Cobra command definitions
      root.go              # Root command, global flags (--config, --verbose, --dry-run)
      scan.go              # scan subcommand (stub -- prints "not implemented")
      rotate.go            # rotate subcommand (stub)
      status.go            # status subcommand (stub)
      history.go           # history subcommand (stub)
    config/
      config.go            # Config struct definition, koanf loading
      validation.go        # Validation rules (required fields, valid types)
      defaults.go          # Default values for optional config
    envfile/
      reader.go            # Line-by-line .env file parser
      writer.go            # Atomic .env file writer (temp + sync + rename)
      types.go             # EnvFile struct (lines, key-value index)
    docker/
      manager.go           # DockerManager interface definition
      client.go            # SDK-backed implementation
      compose.go           # Compose file parsing for depends_on
      health.go            # Health check polling logic
  rotator.example.yml      # Example configuration
  go.mod
  go.sum
```

### Pattern 1: CLI with Cobra

**What:** Root command with four subcommands, global persistent flags.
**When:** Application entry point.

```go
// internal/cli/root.go
package cli

import (
    "github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
    var cfgFile string
    var verbose bool

    rootCmd := &cobra.Command{
        Use:   "rotator",
        Short: "Secret rotation for self-hosted Docker environments",
        Long:  `Rotator discovers, rotates, and manages secrets in Docker Compose environments.`,
    }

    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./rotator.yml)")
    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

    rootCmd.AddCommand(NewScanCmd())
    rootCmd.AddCommand(NewRotateCmd())
    rootCmd.AddCommand(NewStatusCmd())
    rootCmd.AddCommand(NewHistoryCmd())

    return rootCmd
}
```

### Pattern 2: Config Loading with koanf

**What:** YAML file + env var overlay + optional file (zero-config mode).
**When:** Application startup, before any command logic.

```go
// internal/config/config.go
package config

import (
    "github.com/knadh/koanf/v2"
    "github.com/knadh/koanf/parsers/yaml"
    "github.com/knadh/koanf/providers/env"
    "github.com/knadh/koanf/providers/file"
)

type Config struct {
    MasterKeyEnv string          `koanf:"master_key_env"`
    Secrets      []SecretConfig  `koanf:"secrets"`
    Notifications []NotifyConfig `koanf:"notifications"`
}

type SecretConfig struct {
    Name       string            `koanf:"name"`
    Type       string            `koanf:"type"`       // mysql, postgres, redis, generic
    EnvKey     string            `koanf:"env_key"`
    EnvFile    string            `koanf:"env_file"`
    EnvFiles   []string          `koanf:"env_files"`  // multi-file support (DISC-04)
    Containers []string          `koanf:"containers"`
    Provider   map[string]string `koanf:"provider"`
    Schedule   string            `koanf:"schedule"`
    Length     int               `koanf:"length"`
}

func Load(path string) (*Config, error) {
    k := koanf.New(".")

    // Load YAML file if it exists (optional for DISC-03 zero-config)
    if path != "" {
        if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
            return nil, fmt.Errorf("loading config %s: %w", path, err)
        }
    }

    // Overlay with env vars prefixed ROTATOR_
    k.Load(env.Provider("ROTATOR_", ".", func(s string) string {
        return strings.Replace(strings.ToLower(
            strings.TrimPrefix(s, "ROTATOR_")), "_", ".", -1)
    }), nil)

    var cfg Config
    if err := k.Unmarshal("", &cfg); err != nil {
        return nil, fmt.Errorf("unmarshalling config: %w", err)
    }

    if err := validate(&cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

### Pattern 3: Line-Level .env File Editing

**What:** Parse .env as lines preserving comments, blanks, and quoting. Modify only the target key. Never reserialize.
**When:** Every .env read/write operation.

```go
// internal/envfile/types.go
package envfile

// Line represents a single line in an .env file.
// Comments and blank lines have Key == "".
type Line struct {
    Raw     string // Original line text (preserved exactly)
    Key     string // Parsed key (empty for comments/blanks)
    Value   string // Parsed value (unquoted)
    Quoted  string // Quote style: "", "'", "\""
    Comment bool   // True if this is a comment line
}

// EnvFile represents a parsed .env file that preserves formatting.
type EnvFile struct {
    Path  string
    Lines []Line
    index map[string]int // key -> line index for O(1) lookup
}
```

```go
// internal/envfile/reader.go

// Read parses an .env file preserving all formatting.
func Read(path string) (*EnvFile, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    ef := &EnvFile{Path: path, index: make(map[string]int)}
    for i, raw := range strings.Split(string(data), "\n") {
        line := parseLine(raw)
        ef.Lines = append(ef.Lines, line)
        if line.Key != "" {
            ef.index[line.Key] = i
        }
    }
    return ef, nil
}

// parseLine extracts key, value, quote style from a single line.
// Handles: KEY=VALUE, KEY="VALUE", KEY='VALUE', # comment, blank
func parseLine(raw string) Line {
    trimmed := strings.TrimSpace(raw)
    if trimmed == "" {
        return Line{Raw: raw}
    }
    if strings.HasPrefix(trimmed, "#") {
        return Line{Raw: raw, Comment: true}
    }
    // Parse KEY=VALUE with optional quotes
    idx := strings.IndexByte(trimmed, '=')
    if idx < 0 {
        return Line{Raw: raw} // malformed, preserve as-is
    }
    key := strings.TrimSpace(trimmed[:idx])
    val := trimmed[idx+1:]
    quoted := ""
    if len(val) >= 2 {
        if (val[0] == '"' && val[len(val)-1] == '"') ||
           (val[0] == '\'' && val[len(val)-1] == '\'') {
            quoted = string(val[0])
            val = val[1 : len(val)-1]
        }
    }
    return Line{Raw: raw, Key: key, Value: val, Quoted: quoted}
}
```

### Pattern 4: Atomic .env File Writes

**What:** Write to temp file in same directory, fsync, rename. Preserve file permissions.
**When:** Every .env file update.

```go
// internal/envfile/writer.go

// Set updates a key's value, preserving the line's formatting.
func (ef *EnvFile) Set(key, newValue string) {
    if idx, ok := ef.index[key]; ok {
        line := &ef.Lines[idx]
        line.Value = newValue
        // Reconstruct raw line preserving quote style
        if line.Quoted != "" {
            line.Raw = fmt.Sprintf("%s=%s%s%s", line.Key, line.Quoted, newValue, line.Quoted)
        } else {
            line.Raw = fmt.Sprintf("%s=%s", line.Key, newValue)
        }
    }
}

// WriteAtomic writes the .env file atomically via temp-file + sync + rename.
func (ef *EnvFile) WriteAtomic() error {
    // 1. Preserve original permissions
    info, err := os.Stat(ef.Path)
    if err != nil {
        return fmt.Errorf("stat %s: %w", ef.Path, err)
    }

    // 2. Create temp file in SAME directory (required for atomic rename)
    dir := filepath.Dir(ef.Path)
    tmp, err := os.CreateTemp(dir, ".env.tmp.*")
    if err != nil {
        return fmt.Errorf("create temp: %w", err)
    }
    tmpPath := tmp.Name()
    defer os.Remove(tmpPath) // cleanup on any error

    // 3. Write all lines
    var buf strings.Builder
    for i, line := range ef.Lines {
        buf.WriteString(line.Raw)
        if i < len(ef.Lines)-1 {
            buf.WriteByte('\n')
        }
    }
    if _, err := tmp.WriteString(buf.String()); err != nil {
        tmp.Close()
        return fmt.Errorf("write temp: %w", err)
    }

    // 4. Sync to disk before rename
    if err := tmp.Sync(); err != nil {
        tmp.Close()
        return fmt.Errorf("sync temp: %w", err)
    }
    tmp.Close()

    // 5. Match original permissions
    if err := os.Chmod(tmpPath, info.Mode()); err != nil {
        return fmt.Errorf("chmod temp: %w", err)
    }

    // 6. Atomic rename
    if err := os.Rename(tmpPath, ef.Path); err != nil {
        return fmt.Errorf("rename %s -> %s: %w", tmpPath, ef.Path, err)
    }

    return nil
}
```

### Pattern 5: Docker Manager Interface

**What:** Narrow interface wrapping Docker SDK. Enables mocking in tests.
**When:** All container interactions.

```go
// internal/docker/manager.go
package docker

import (
    "context"
    "time"
)

// Container holds the subset of container info the rotator needs.
type Container struct {
    ID      string
    Name    string
    Image   string
    Status  string
    Health  string            // "healthy", "unhealthy", "starting", "none"
    Labels  map[string]string
    EnvVars []string          // from container config
}

// ContainerFilter defines criteria for listing containers.
type ContainerFilter struct {
    Names  []string
    Labels map[string]string
}

// Manager defines the contract for Docker operations.
// All container interactions go through this interface.
type Manager interface {
    ListContainers(ctx context.Context, filter ContainerFilter) ([]Container, error)
    InspectContainer(ctx context.Context, id string) (*Container, error)
    StopContainer(ctx context.Context, id string, timeout time.Duration) error
    StartContainer(ctx context.Context, id string) error
    RestartContainer(ctx context.Context, id string, timeout time.Duration) error
    WaitHealthy(ctx context.Context, id string, timeout time.Duration) error
}
```

```go
// internal/docker/client.go -- SDK implementation

type SDKClient struct {
    cli *client.Client
}

func NewSDKClient() (*SDKClient, error) {
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return nil, err
    }
    return &SDKClient{cli: cli}, nil
}

func (s *SDKClient) WaitHealthy(ctx context.Context, id string, timeout time.Duration) error {
    deadline := time.After(timeout)
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-deadline:
            return fmt.Errorf("container %s did not become healthy within %s", id, timeout)
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            resp, err := s.cli.ContainerInspect(ctx, id)
            if err != nil {
                return err
            }
            if resp.State == nil {
                continue
            }
            // No health check configured -- consider running as healthy
            if resp.State.Health == nil {
                if resp.State.Running {
                    return nil
                }
                continue
            }
            if resp.State.Health.Status == "healthy" {
                return nil
            }
        }
    }
}
```

### Pattern 6: Dependency-Ordered Restart

**What:** Parse `depends_on` from compose files via compose-spec/compose-go. Topological sort. Restart databases first, then apps.
**When:** Container restart after secret rotation.

```go
// internal/docker/compose.go

import (
    "github.com/compose-spec/compose-go/v2/cli"
)

// LoadDependencyOrder parses a compose file and returns service names
// in dependency order (dependencies first).
func LoadDependencyOrder(composePath string) ([]string, error) {
    options, err := cli.NewProjectOptions(
        []string{composePath},
        cli.WithOsEnv,
        cli.WithDotEnv,
    )
    if err != nil {
        return nil, err
    }
    project, err := options.LoadProject(context.Background())
    if err != nil {
        return nil, err
    }

    // Build adjacency list from depends_on
    deps := make(map[string][]string)
    for _, svc := range project.Services {
        for dep := range svc.DependsOn {
            deps[svc.Name] = append(deps[svc.Name], dep)
        }
    }

    // Topological sort (Kahn's algorithm)
    return topoSort(deps)
}
```

### Anti-Patterns to Avoid

- **God package:** Do NOT put CLI, config, envfile, and Docker code in a single package. Each is independently testable with its own interface.
- **Direct Docker SDK calls throughout:** Wrap in the Manager interface. Only `internal/docker/client.go` imports the Docker SDK.
- **Using godotenv to round-trip .env files:** It destroys comments, reorders keys, and normalizes quoting. Use line-level editing.
- **os.WriteFile for .env updates:** Truncates before writing -- a crash mid-write corrupts the file. Always use temp + sync + rename.
- **Hardcoding compose file paths:** Use config to specify compose file locations, or discover via `COMPOSE_FILE` env var.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CLI command parsing | Custom arg parser | spf13/cobra | Subcommands, help text, shell completions, flag parsing -- all solved |
| YAML config loading | Custom YAML parser | koanf + yaml parser | Layered config (file + env + flags), struct unmarshalling, validation hooks |
| Compose file parsing | Custom YAML struct for docker-compose.yml | compose-spec/compose-go/v2 | Full compose spec support including extends, variable interpolation, depends_on |
| Docker container ops | Shell exec (`docker restart`) | docker/docker/client SDK | Proper error handling, timeout control, streaming, no platform dependency |
| Topological sort | Ad-hoc ordering | Kahn's algorithm (20 lines) | Correct cycle detection, deterministic output. Small enough to hand-write. |

**Key insight:** The .env file reader/writer IS a hand-roll, but intentionally. No Go library correctly preserves comments, blank lines, and quoting style on round-trip. The implementation is ~100 lines and fully testable.

## Common Pitfalls

### Pitfall 1: Non-Atomic .env Writes

**What goes wrong:** `os.WriteFile` truncates the file before writing. A crash (power loss, SIGKILL, disk full) between truncate and write-complete leaves the .env file empty or partial. All services referencing it break on next restart.
**Why it happens:** `os.WriteFile` is the "obvious" Go API for writing files. Developers don't realize it truncates first.
**How to avoid:** Always write to a temp file in the same directory, call `Sync()`, then `os.Rename()`. Rename is atomic on Linux within the same filesystem.
**Warning signs:** Any use of `os.WriteFile` or `os.Create` on a production .env file.

### Pitfall 2: Viper Key Lowercasing

**What goes wrong:** Viper forces all config keys to lowercase. A config key `MYSQL_PASSWORD` becomes `mysql_password`. When the tool tries to match env var names in .env files, the case mismatch causes failures.
**Why it happens:** Viper was designed for case-insensitive config, not for managing case-sensitive env var names.
**How to avoid:** Use koanf, which preserves key casing. Already decided in stack research.
**Warning signs:** Using Viper anywhere in this project.

### Pitfall 3: Container Restart Without Readiness

**What goes wrong:** Restarting a DB container and immediately starting dependent app containers. The app crashes because the DB isn't accepting connections yet. Docker Compose `depends_on` only waits for "running" state, not "ready."
**Why it happens:** Developers test with warm containers that restart in <1s. Cold starts or large DBs take 5-30s.
**How to avoid:** After restarting a dependency, poll health check status (or TCP connect) before restarting dependents. Implement configurable timeouts.
**Warning signs:** `RestartContainer` called in a loop with no wait between dependency and dependent.

### Pitfall 4: Ignoring .env Quoting Styles

**What goes wrong:** An .env file has `DB_PASSWORD="p@ss with spaces"`. The tool strips quotes when reading and doesn't restore them when writing. Docker Compose then parses the value differently, breaking authentication.
**Why it happens:** Naive parsing treats quotes as value delimiters but doesn't preserve the style.
**How to avoid:** The `Line` struct stores the original `Quoted` style and restores it on write. Test with quoted values containing spaces, `#` characters, and `$` signs.
**Warning signs:** Tests only use simple `KEY=value` patterns.

### Pitfall 5: Compose File Discovery

**What goes wrong:** Tool assumes `docker-compose.yml` is in the same directory as `.env`. Modern Docker Compose uses `compose.yaml`, `compose.yml`, `docker-compose.yaml`, or `docker-compose.yml`, and can be in a parent directory.
**Why it happens:** Hard-coded path assumption.
**How to avoid:** Let the user specify compose path in config. For auto-discovery, check all four filenames. compose-spec/compose-go handles this via `cli.NewProjectOptions` defaults.
**Warning signs:** Hard-coded `docker-compose.yml` string in code.

## Code Examples

All examples above in Architecture Patterns are verified patterns. Additional reference:

### Minimal main.go

```go
// cmd/rotator/main.go
package main

import (
    "os"
    "github.com/giulio/secret-rotator/internal/cli"
)

func main() {
    if err := cli.NewRootCmd().Execute(); err != nil {
        os.Exit(1)
    }
}
```

### Config Validation

```go
// internal/config/validation.go
package config

import "fmt"

var validTypes = map[string]bool{
    "mysql": true, "postgres": true, "redis": true, "generic": true,
}

func validate(cfg *Config) error {
    for i, s := range cfg.Secrets {
        if s.Name == "" {
            return fmt.Errorf("secrets[%d]: name is required", i)
        }
        if s.Type != "" && !validTypes[s.Type] {
            return fmt.Errorf("secrets[%d] %q: invalid type %q (valid: mysql, postgres, redis, generic)", i, s.Name, s.Type)
        }
        if s.EnvKey == "" {
            return fmt.Errorf("secrets[%d] %q: env_key is required", i, s.Name)
        }
        if s.EnvFile == "" && len(s.EnvFiles) == 0 {
            return fmt.Errorf("secrets[%d] %q: env_file or env_files is required", i, s.Name)
        }
    }
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Viper for all Go config | koanf for case-sensitive config | 2023+ | Viper still dominant but koanf preferred when key casing matters |
| godotenv for .env round-trip | Line-level text editing | Always (godotenv never claimed to preserve format) | Must build custom reader/writer (~100 lines) |
| docker/go-sdk (new high-level) | docker/docker/client (battle-tested) | go-sdk published Jul 2025 | go-sdk is too new; stick with docker/docker/client for stability |
| Cobra v1.8 | Cobra v1.10.2 | Dec 2024 | Minor improvements; v2 does NOT exist as a released module |

**Deprecated/outdated:**
- **Cobra v2**: Does not exist. Research docs mentioning "cobra v2.x" are incorrect. Latest is v1.10.2.
- **lib/pq**: Deprecated by its maintainer in favor of pgx. Not relevant to Phase 1 but noted for later phases.
- **fsouza/go-dockerclient**: Community alternative that lags behind official SDK. Do not use.

## Open Questions

1. **Compose file auto-discovery scope**
   - What we know: compose-spec/compose-go handles standard filenames
   - What's unclear: Should the tool auto-discover compose files in subdirectories, or only in the configured path?
   - Recommendation: Config-first (user specifies paths), auto-discovery as convenience for zero-config mode scanning only the current directory

2. **Multi-file .env value conflicts**
   - What we know: A secret can appear in both `.env` and `.env.local`
   - What's unclear: When updating, should both files be updated, or only the one where the key appears?
   - Recommendation: Update ALL files containing the key. Warn if the same key has different values across files.

3. **Docker socket availability in tests**
   - What we know: DockerManager interface enables mocking
   - What's unclear: Should Phase 1 include any integration tests against a real Docker daemon?
   - Recommendation: No. All Phase 1 Docker tests use mock Manager. Integration tests with real Docker come in later phases when container restart logic is exercised end-to-end.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.9.x |
| Config file | None needed -- `go test ./...` works out of the box |
| Quick run command | `go test ./internal/... -short -count=1` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DISC-03 | Config loads with no config file (zero-config) | unit | `go test ./internal/config/ -run TestLoadNoFile -count=1` | Wave 0 |
| DISC-04 | EnvFile reads multiple .env files | unit | `go test ./internal/envfile/ -run TestReadMultiple -count=1` | Wave 0 |
| DIST-03 | Config loads and validates rotator.yml | unit | `go test ./internal/config/ -run TestLoadYAML -count=1` | Wave 0 |
| INFR-01 | DockerManager lists/inspects/restarts containers; dependency ordering | unit (mock) | `go test ./internal/docker/ -run TestRestart -count=1` | Wave 0 |
| ROT-03 | EnvFile writes atomically (temp+rename) | unit | `go test ./internal/envfile/ -run TestAtomicWrite -count=1` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/... -short -count=1`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green + `go vet ./...` + no race conditions

### Wave 0 Gaps

- [ ] `internal/config/config_test.go` -- covers DISC-03, DIST-03
- [ ] `internal/envfile/reader_test.go` -- covers DISC-04, ROT-03 (read side)
- [ ] `internal/envfile/writer_test.go` -- covers ROT-03 (atomic write)
- [ ] `internal/docker/manager_test.go` -- covers INFR-01 (mock-based)
- [ ] `internal/docker/compose_test.go` -- covers INFR-01 (dependency ordering)
- [ ] Test fixtures: sample `.env` files with comments, quotes, multiline; sample `rotator.yml`; sample `docker-compose.yml` with depends_on

## Sources

### Primary (HIGH confidence)

- [spf13/cobra releases](https://github.com/spf13/cobra/releases) -- v1.10.2 is latest (Dec 2024), v2 does not exist
- [knadh/koanf v2.3.0 on pkg.go.dev](https://pkg.go.dev/github.com/knadh/koanf/v2@v2.3.0) -- published Sep 2025, YAML + env + file providers
- [docker/docker/client on pkg.go.dev](https://pkg.go.dev/github.com/docker/docker/client) -- ContainerList, ContainerInspect, ContainerStop, ContainerStart, ContainerRestart signatures
- [compose-spec/compose-go on GitHub](https://github.com/compose-spec/compose-go) -- Go reference library for Compose file parsing
- [natefinch/atomic on GitHub](https://github.com/natefinch/atomic) -- atomic file write pattern reference
- [Michael Stapelberg: Atomically writing files in Go](https://michael.stapelberg.ch/posts/2017-01-28-golang_atomically_writing/) -- temp + sync + rename pattern

### Secondary (MEDIUM confidence)

- [Docker Engine v27 release notes](https://docs.docker.com/engine/release-notes/27/) -- API compatibility
- [alexwlchan: Create a file atomically in Go](https://alexwlchan.net/notes/2026/go-atomicfile/) -- recent (2026) atomic write guidance

### Tertiary (LOW confidence)

- WebSearch result claiming Cobra v2.3.0 exists -- **INCORRECT, verified as v1.10.2** via official releases page

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified against official sources with correct version numbers
- Architecture: HIGH -- patterns follow established Go conventions (internal/, interface-driven, narrow Docker wrapper)
- Pitfalls: HIGH -- atomic write, Viper key-casing, restart ordering all documented with specific prevention strategies
- .env parsing: MEDIUM -- custom implementation needed; no library handles round-trip preservation. Implementation is straightforward but must be thoroughly tested with edge cases.

**Research date:** 2026-03-27
**Valid until:** 2026-04-27 (stable domain, slow-moving libraries)
