# Phase 2: Discovery and Crypto - Research

**Researched:** 2026-03-27
**Domain:** Secret discovery via env var pattern matching, password strength auditing, AES-256-GCM encryption with Argon2id key derivation
**Confidence:** HIGH

## Summary

Phase 2 builds two independent subsystems on the Phase 1 foundation: (1) a secret discovery engine that scans `.env` files to identify secrets by naming patterns and audit their strength, powering the `rotator scan` CLI command; and (2) an encrypted secret store using AES-256-GCM with Argon2id key derivation for rotation history, powering the `rotator history` CLI command.

Both subsystems are well-served by Go stdlib and established patterns. The discovery engine needs no external dependencies -- pattern matching against env var names (suffixes like `_PASSWORD`, `_SECRET`, `_KEY`, `_TOKEN`) is straightforward string matching, and strength auditing uses entropy calculation plus checks against known weak/default passwords. The crypto subsystem uses `golang.org/x/crypto/argon2` for key derivation and `crypto/aes` + `crypto/cipher` (stdlib) for AES-256-GCM encryption, both thoroughly documented and battle-tested.

The two subsystems have no mutual dependency and can be built in either order. The discovery engine depends on Phase 1's `envfile.Read()` and `config.Config`. The crypto/history subsystem is self-contained, storing encrypted records in a JSON file on disk.

**Primary recommendation:** Build discovery engine first (immediate user value via `rotator scan`), then crypto + history. Use hand-rolled pattern matching for secret detection (no external library needed). Use stdlib crypto with Argon2id -- no external encryption libraries.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DISC-01 | Tool auto-scans `.env` files and identifies secrets by naming patterns | Pattern matching against known suffixes (_PASSWORD, _SECRET, _KEY, _TOKEN, etc.) using the existing envfile.Keys() API |
| DISC-02 | Tool audits password strength and flags weak, default, or short passwords | Entropy-based scoring with length checks, character class analysis, and a built-in list of common default passwords |
| CLI-01 | `rotator scan` command discovers and reports secrets with strength audit | Wire discovery engine into existing scan.go Cobra stub, output table format |
| CLI-04 | `rotator history` command shows rotation audit log | Wire encrypted store into existing history.go Cobra stub, decrypt and display records |
| INFR-02 | Old secrets encrypted at rest using AES-256-GCM with Argon2id key derivation | Argon2id (RFC 9106 params) for key derivation from master passphrase, AES-256-GCM with random nonces for encryption |
</phase_requirements>

## Standard Stack

### Core (no new external dependencies needed for discovery)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/crypto/argon2 | latest | Argon2id key derivation from master passphrase | RFC 9106 winner, Go semi-stdlib, used by OWASP-recommended implementations |
| crypto/aes + crypto/cipher (stdlib) | Go 1.25 | AES-256-GCM authenticated encryption | Go stdlib, hardware-accelerated (AES-NI), zero external deps |
| crypto/rand (stdlib) | Go 1.25 | Cryptographically secure random nonces and salts | Go stdlib CSPRNG |
| encoding/json (stdlib) | Go 1.25 | History file serialization | JSON is human-debuggable and sufficient for audit log |

### Supporting (already in go.mod)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/envfile | Phase 1 | Read .env files for secret scanning | Discovery engine uses envfile.Read() and Keys() |
| internal/config | Phase 1 | Load config for secret definitions | Scan merges config-defined secrets with auto-discovered ones |
| internal/docker | Phase 1 | List containers for label-based discovery | Optional enrichment: which containers reference each secret |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled pattern matching | nbutton23/zxcvbn-go | Archived/unmaintained, heavy dependency for simple pattern matching |
| Hand-rolled entropy scoring | wagslane/go-password-validator | Lightweight but lacks default/common password detection; we need both |
| JSON history file | SQLite (modernc.org/sqlite) | Overkill for append-only audit log; adds CGO or large pure-Go dep |
| AES-256-GCM | age encryption | Out of scope per project requirements -- we manage our own crypto |

### New Dependency

```bash
go get golang.org/x/crypto@latest
```

This is already a transitive dependency (compose-go pulls it in), but it should be promoted to a direct dependency for the argon2 import.

## Architecture Patterns

### Recommended Project Structure

```
internal/
  discovery/
    scanner.go         # Secret scanner: pattern matching + env file scanning
    patterns.go        # Known secret name patterns and classification
    strength.go        # Password strength auditing (entropy, length, defaults)
    scanner_test.go    # Tests for scanner
    strength_test.go   # Tests for strength auditor
  crypto/
    crypto.go          # Argon2id key derivation + AES-256-GCM encrypt/decrypt
    crypto_test.go     # Round-trip tests, known test vectors
  history/
    store.go           # Encrypted history file store (read/write/append)
    types.go           # HistoryEntry, HistoryFile types
    store_test.go      # Tests for store operations
  cli/
    scan.go            # Updated: wires discovery engine into cobra command
    history.go         # Updated: wires history store into cobra command
```

### Pattern 1: Secret Detection by Naming Convention

**What:** Classify env var keys as secrets based on suffix/substring matching against known patterns.
**When to use:** Always -- this is the primary discovery mechanism for zero-config mode.

```go
// Source: domain knowledge from secret scanning tools (GitGuardian, trufflehog patterns)

// SecretPattern defines a pattern that identifies a secret env var.
type SecretPattern struct {
    Suffix  string // e.g., "_PASSWORD", "_SECRET"
    Type    string // e.g., "password", "api_key", "token"
    Exact   []string // exact matches, e.g., "SECRET_KEY", "JWT_SECRET"
}

var DefaultPatterns = []SecretPattern{
    {Suffix: "_PASSWORD", Type: "password"},
    {Suffix: "_PASSWD", Type: "password"},
    {Suffix: "_SECRET", Type: "secret"},
    {Suffix: "_KEY", Type: "api_key"},
    {Suffix: "_TOKEN", Type: "token"},
    {Suffix: "_API_KEY", Type: "api_key"},
    {Suffix: "_APIKEY", Type: "api_key"},
    {Suffix: "_AUTH", Type: "credential"},
    {Suffix: "_CREDENTIAL", Type: "credential"},
    {Suffix: "_PRIVATE_KEY", Type: "private_key"},
}

// Exact matches for well-known secret names
var ExactPatterns = []string{
    "SECRET_KEY", "JWT_SECRET", "SESSION_SECRET",
    "DATABASE_URL", "REDIS_URL", "MONGO_URI",
}
```

### Pattern 2: Password Strength Scoring

**What:** Score password strength using entropy calculation, length checks, and default/common password detection.
**When to use:** During scan to flag weak secrets.

```go
// Strength levels returned by the auditor
type Strength int
const (
    StrengthWeak     Strength = iota // Score 0-1: immediate risk
    StrengthFair                      // Score 2: below recommended
    StrengthGood                      // Score 3: acceptable
    StrengthStrong                    // Score 4: excellent
)

// StrengthResult holds the audit result for a single secret value.
type StrengthResult struct {
    Score    Strength
    Entropy  float64
    Length   int
    Issues   []string // human-readable warnings
}
```

Scoring criteria:
- **Length:** <8 = weak, 8-15 = fair, 16-31 = good, 32+ = strong
- **Character classes:** count of {lowercase, uppercase, digits, symbols}; <2 classes = penalty
- **Entropy:** calculated as `length * log2(charset_size)`; <28 bits = weak, <36 = fair, <60 = good, 60+ = strong
- **Default detection:** check against list of known defaults (e.g., "changeme", "password", "secret", "admin", "root", "default", "test", "example", "12345678")
- **Repetition:** all-same-char or simple sequence detection

### Pattern 3: AES-256-GCM Encryption with Argon2id

**What:** Derive a 256-bit key from user passphrase via Argon2id, encrypt data with AES-256-GCM.
**When to use:** Encrypting history entries at rest.

```go
// Source: golang.org/x/crypto/argon2 docs, Go crypto/cipher docs

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "io"
    "golang.org/x/crypto/argon2"
)

// Argon2id parameters per RFC 9106 Section 7.3
const (
    argonTime    = 1
    argonMemory  = 64 * 1024  // 64 MiB
    argonThreads = 4
    argonKeyLen  = 32          // AES-256
    saltLen      = 16
)

// DeriveKey derives a 256-bit AES key from passphrase + salt using Argon2id.
func DeriveKey(passphrase []byte, salt []byte) []byte {
    return argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
// Returns: salt (16) + nonce (12) + ciphertext (variable)
func Encrypt(plaintext, passphrase []byte) ([]byte, error) {
    salt := make([]byte, saltLen)
    if _, err := io.ReadFull(rand.Reader, salt); err != nil {
        return nil, err
    }
    key := DeriveKey(passphrase, salt)

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonce := make([]byte, gcm.NonceSize()) // 12 bytes
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }

    ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
    // Format: salt + nonce + ciphertext (includes GCM auth tag)
    result := make([]byte, 0, saltLen+len(nonce)+len(ciphertext))
    result = append(result, salt...)
    result = append(result, nonce...)
    result = append(result, ciphertext...)
    return result, nil
}
```

### Pattern 4: Encrypted History File Format

**What:** JSON file with base64-encoded encrypted entries.
**When to use:** Storing and retrieving rotation audit log.

```go
// HistoryEntry is the plaintext structure encrypted per-entry.
type HistoryEntry struct {
    SecretName string    `json:"secret_name"`
    RotatedAt  time.Time `json:"rotated_at"`
    OldValue   string    `json:"old_value"`
    NewHash    string    `json:"new_hash"` // SHA-256 of new value (for verification, not storage)
    Status     string    `json:"status"`   // "success", "failed", "rolled_back"
    Details    string    `json:"details"`
}

// HistoryFile is the on-disk format.
type HistoryFile struct {
    Version int              `json:"version"` // Format version for future migration
    Entries []EncryptedEntry `json:"entries"`
}

// EncryptedEntry is the on-disk representation of one history entry.
type EncryptedEntry struct {
    Data      string `json:"data"`       // base64(salt + nonce + ciphertext)
    CreatedAt string `json:"created_at"` // ISO 8601 timestamp (plaintext for listing without decrypt)
}
```

**Design decision:** Each entry is encrypted independently (not the whole file). This enables:
- Append without re-encrypting everything
- Streaming decryption for large histories
- Individual entry corruption does not lose the entire history

**File location:** `.rotator/history.json` in the project directory (same directory as `.env` files).

### Pattern 5: Scan Output Format

**What:** Table-formatted output showing discovered secrets with type, container references, and strength.
**When to use:** `rotator scan` command output.

```
SECRET                    TYPE       STRENGTH    CONTAINERS    ISSUES
MYSQL_ROOT_PASSWORD       password   weak        mysql, app    Length < 8, common default
REDIS_PASSWORD            password   strong      redis, app    -
JWT_SECRET                secret     fair        app           No special characters
API_KEY                   api_key    good        app, worker   -
```

### Anti-Patterns to Avoid

- **Regex-based secret VALUE detection:** Do NOT try to detect secrets by their values (e.g., "looks like a base64 string"). This produces too many false positives. Match on KEY NAMES only.
- **Loading the entire password dictionary:** Do NOT embed large wordlists. A small list of ~50 known defaults is sufficient. This is a scan tool, not a penetration tester.
- **Re-deriving the key for every decrypt:** Cache the derived key in memory for the duration of the command. Argon2id is intentionally slow (that is its purpose). Derive once, use for all entries.
- **Encrypting the whole history file:** Encrypt per-entry, not the whole file. Whole-file encryption requires re-encrypting on every append and loses atomicity.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Key derivation | SHA-256 of passphrase | Argon2id via golang.org/x/crypto/argon2 | SHA-256 is not a KDF; trivially brute-forced |
| Authenticated encryption | AES-CBC + separate HMAC | AES-GCM via crypto/cipher.NewGCM | GCM provides authentication built-in; CBC+HMAC is easy to get wrong (padding oracle attacks) |
| Random nonce generation | time.Now().UnixNano() or counter | crypto/rand.Read() | Predictable nonces break GCM entirely |
| Password generation | math/rand | crypto/rand with configurable charset | math/rand is not cryptographically secure |
| Secret name patterns | Custom regex engine | Simple string suffix matching | Suffix matching is O(1) per pattern, simpler to maintain, no regex compilation |

**Key insight:** The crypto domain has exactly one correct approach for each primitive. Use Argon2id for KDF, AES-256-GCM for AEAD, crypto/rand for randomness. There are no legitimate alternatives for a new project in 2026.

## Common Pitfalls

### Pitfall 1: Nonce Reuse in AES-GCM
**What goes wrong:** Reusing a nonce with the same key completely breaks GCM -- leaks the authentication key and allows forgeries.
**Why it happens:** Using deterministic nonces (counters, timestamps) or not generating fresh random nonces per encryption.
**How to avoid:** Always use `crypto/rand.Read()` to generate a fresh 12-byte nonce for each `Seal()` call. With random nonces, stay under 2^32 (~4 billion) encryptions per key. For a rotation history tool, this limit is unreachable.
**Warning signs:** `nonce++` or `nonce = hash(something)` anywhere in crypto code.

### Pitfall 2: Master Key from Managed .env File
**What goes wrong:** The master key passphrase is stored in the same `.env` file the tool manages. During rotation, the tool may overwrite or corrupt the file, losing access to the encryption key.
**Why it happens:** Users put everything in `.env` for convenience.
**How to avoid:** Master key must come from a SEPARATE source: `ROTATOR_MASTER_KEY` environment variable (set in shell profile or Docker env), a separate file (e.g., `.rotator/master.key`), or interactive prompt. NEVER from a managed `.env` file. The config field `master_key_env` already supports this pattern.
**Warning signs:** Master key env var name appearing in any scanned `.env` file.

### Pitfall 3: Logging Secret Values During Scan
**What goes wrong:** The scan command outputs or logs actual secret values in the strength audit.
**Why it happens:** Debug logging includes full context; developers forget to redact.
**How to avoid:** The strength auditor receives the value for analysis but NEVER returns or logs it. Only the StrengthResult (score, entropy, issues) is surfaced. Mask values in any output: show only first 2 chars + asterisks.
**Warning signs:** Secret values appearing in test output, log files, or terminal scrollback.

### Pitfall 4: Ignoring _FILE Suffixed Variables
**What goes wrong:** Docker secrets pattern uses `DB_PASSWORD_FILE=/run/secrets/db_password` instead of `DB_PASSWORD=actual_value`. The scanner misses these because the value looks like a file path, not a secret.
**Why it happens:** The `_FILE` convention is Docker-specific and not well-known.
**How to avoid:** When a key matches a secret pattern but ends with `_FILE`, flag it as "file-referenced secret" rather than trying to audit the path as a password.
**Warning signs:** `_FILE` suffix variables being scored as "strong" because file paths have high entropy.

### Pitfall 5: Slow CLI Due to Argon2id on Every History Read
**What goes wrong:** `rotator history` takes 200ms+ per entry because Argon2id is re-derived for each one.
**Why it happens:** Argon2id with RFC 9106 params (64 MiB memory) takes ~50-100ms per derivation. Deriving for each entry in a 50-entry history = 2.5-5 seconds.
**How to avoid:** Derive the key ONCE at command start, hold in memory for the command duration, clear from memory after. Use a `KeyRing` or similar struct that derives on first use and caches.
**Warning signs:** History command getting progressively slower as entries accumulate.

## Code Examples

### Secret Scanner Implementation

```go
// Source: project-specific pattern based on envfile.Keys() API from Phase 1

func (s *Scanner) ScanFile(ef *envfile.EnvFile) []DiscoveredSecret {
    var secrets []DiscoveredSecret
    for _, key := range ef.Keys() {
        if st := s.classifyKey(key); st != "" {
            value, _ := ef.Get(key)
            strength := s.auditStrength(value)
            secrets = append(secrets, DiscoveredSecret{
                Key:       key,
                Type:      st,
                Source:    ef.Path,
                Strength:  strength,
            })
        }
    }
    return secrets
}

func (s *Scanner) classifyKey(key string) string {
    upper := strings.ToUpper(key)
    // Check exact matches first
    for _, exact := range s.exactPatterns {
        if upper == exact {
            return "secret"
        }
    }
    // Check suffix patterns
    for _, p := range s.suffixPatterns {
        if strings.HasSuffix(upper, p.Suffix) {
            return p.Type
        }
    }
    return ""
}
```

### Argon2id + AES-GCM Decrypt

```go
// Source: golang.org/x/crypto/argon2, crypto/cipher docs

func Decrypt(data, passphrase []byte) ([]byte, error) {
    if len(data) < saltLen+12 { // salt + minimum nonce
        return nil, ErrDataTooShort
    }

    salt := data[:saltLen]
    key := DeriveKey(passphrase, salt)

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonceSize := gcm.NonceSize()
    if len(data) < saltLen+nonceSize {
        return nil, ErrDataTooShort
    }

    nonce := data[saltLen : saltLen+nonceSize]
    ciphertext := data[saltLen+nonceSize:]

    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, ErrDecryptionFailed // wrong passphrase or corrupted data
    }
    return plaintext, nil
}
```

### Default Password List

```go
// Source: OWASP common passwords, Docker image default credentials

var CommonDefaults = map[string]bool{
    "":             true,
    "password":     true,
    "changeme":     true,
    "secret":       true,
    "admin":        true,
    "root":         true,
    "default":      true,
    "test":         true,
    "example":      true,
    "12345678":     true,
    "123456789":    true,
    "1234567890":   true,
    "qwerty":       true,
    "letmein":      true,
    "welcome":      true,
    "monkey":       true,
    "docker":       true,
    "mysql":        true,
    "postgres":     true,
    "redis":        true,
    "guest":        true,
    "pass":         true,
    "password1":    true,
    "abc123":       true,
    "p@ssw0rd":     true,
    "supersecret":  true,
    "mysecretkey":  true,
    "changeit":     true,
    "trustno1":     true,
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PBKDF2/bcrypt for key derivation | Argon2id (RFC 9106) | 2023+ widely adopted | Memory-hard, resists GPU/ASIC attacks; mandatory for new projects |
| AES-CBC + HMAC | AES-GCM (AEAD) | Standard since ~2015 | Single-primitive authenticated encryption; eliminates padding oracle bugs |
| Complex regex for secret detection | Suffix/prefix pattern matching | Current best practice | Simpler, fewer false positives, easier to maintain |
| zxcvbn for password strength | Entropy + default detection | zxcvbn-go archived | Lighter, no unmaintained dependency, sufficient for env var audit |

**Deprecated/outdated:**
- nbutton23/zxcvbn-go: Archived repository, not maintained. Do not use.
- PBKDF2 for new projects: Argon2id is the recommended replacement per OWASP 2025.
- AES-CBC: Use AES-GCM instead for authenticated encryption.

## Open Questions

1. **History file retention policy**
   - What we know: History will grow indefinitely with automated rotation
   - What's unclear: Should there be a default retention limit?
   - Recommendation: Start with no limit for v1; add `--keep-last N` flag later. The encrypted JSON file handles hundreds of entries fine.

2. **Master key rotation (re-encryption)**
   - What we know: Users may need to change their master passphrase
   - What's unclear: Should Phase 2 support re-encryption, or defer to later?
   - Recommendation: Defer to later phase. Document that changing the passphrase requires a future `rotator rekey` command. For v1, the passphrase is set once.

3. **Container enrichment in scan output**
   - What we know: The scan command should show which containers reference each secret
   - What's unclear: Should this require a running Docker daemon, or parse compose files only?
   - Recommendation: Parse compose files and .env file references without requiring Docker. The scan command should work without Docker running (zero-config, zero-dependency).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify v1.11.1 |
| Config file | None (go test convention) |
| Quick run command | `go test ./internal/discovery/... ./internal/crypto/... ./internal/history/...` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DISC-01 | Scanner identifies secrets by naming patterns | unit | `go test ./internal/discovery/ -run TestScannerClassify -x` | Wave 0 |
| DISC-02 | Strength auditor flags weak/default passwords | unit | `go test ./internal/discovery/ -run TestStrength -x` | Wave 0 |
| CLI-01 | `rotator scan` outputs table with secrets and strength | unit | `go test ./internal/cli/ -run TestScanCmd -x` | Wave 0 |
| CLI-04 | `rotator history` displays decrypted audit log | unit | `go test ./internal/cli/ -run TestHistoryCmd -x` | Wave 0 |
| INFR-02 | AES-256-GCM encrypt/decrypt round-trip with Argon2id | unit | `go test ./internal/crypto/ -run TestEncryptDecrypt -x` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/discovery/... ./internal/crypto/... ./internal/history/...`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps
- [ ] `internal/discovery/scanner_test.go` -- covers DISC-01 (pattern matching)
- [ ] `internal/discovery/strength_test.go` -- covers DISC-02 (strength auditing)
- [ ] `internal/crypto/crypto_test.go` -- covers INFR-02 (encrypt/decrypt round-trip)
- [ ] `internal/history/store_test.go` -- covers CLI-04 (history read/write)
- [ ] New dependency: `go get golang.org/x/crypto@latest` (direct dep for argon2)

## Sources

### Primary (HIGH confidence)
- [golang.org/x/crypto/argon2](https://pkg.go.dev/golang.org/x/crypto/argon2) - IDKey function, RFC 9106 parameter recommendations
- [crypto/cipher - Go Packages](https://pkg.go.dev/crypto/cipher) - AES-GCM NewGCM, Seal, Open API
- [crypto/aes - Go Packages](https://pkg.go.dev/crypto/aes) - AES block cipher construction
- [Go crypto/cipher example_test.go](https://go.dev/src/crypto/cipher/example_test.go) - Official GCM usage examples
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html) - AES-256-GCM + Argon2id recommendations

### Secondary (MEDIUM confidence)
- [AES-256 GCM Encryption Example in Golang (Gist)](https://gist.github.com/kkirsche/e28da6754c39d5e7ea10) - Community AES-GCM pattern reference
- [Encrypt/Decrypt Data in Go with AES-256 - Twilio](https://www.twilio.com/en-us/blog/developers/community/encrypt-and-decrypt-data-in-go-with-aes-256) - Verified AES-GCM nonce handling
- [wagslane/go-password-validator](https://github.com/wagslane/go-password-validator) - Entropy-based password scoring approach (used as design reference, not as dependency)
- [How to Handle Secrets in Go - GitGuardian](https://blog.gitguardian.com/how-to-handle-secrets-in-go/) - Secret naming conventions in Go ecosystem
- [Password Hashing Guide 2025](https://guptadeepak.com/the-complete-guide-to-password-hashing-argon2-vs-bcrypt-vs-scrypt-vs-pbkdf2-2026/) - Argon2id parameter recommendations and comparisons

### Tertiary (LOW confidence)
- [nbutton23/zxcvbn-go](https://github.com/nbutton23/zxcvbn-go) - Evaluated and rejected (archived/unmaintained)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All crypto primitives are Go stdlib or x/crypto; well-documented, stable APIs
- Architecture: HIGH - Pattern matching and per-entry encryption are straightforward; builds directly on Phase 1 envfile API
- Pitfalls: HIGH - Crypto pitfalls (nonce reuse, weak KDF) are well-documented in OWASP and Go docs; discovery pitfalls identified from domain analysis

**Research date:** 2026-03-27
**Valid until:** 2026-04-27 (stable domain; crypto primitives do not change frequently)
