# Architecture

Secret Rotator is built as a hexagonal (ports and adapters) application with a
domain-driven core. Four layers, and every dependency points inwards.

```
internal/
  domain/           model + rules + ports      no I/O, stdlib only
  application/      use cases                  depends on domain only
  infrastructure/   adapters                   implement the domain's ports
  interfaces/cli/   driving adapter            composition root
```

`internal/architecture_test.go` enforces this. It fails the build if the domain
imports a third-party package, if the application reaches for an adapter, or if
an adapter imports the CLI.

## Why

The previous structure had ports and adapters in the right places but the
dependencies the wrong way round. `config.SecretConfig` — a struct whose fields
mirrored the YAML file — was passed to the rotation engine, the scheduler and
the providers, so the model was shaped by the configuration format and changing
one meant changing the other. The engine imported concrete types from four
infrastructure packages. Connection details travelled as a `map[string]string`
that each provider re-parsed with its own defaulting rules.

The rearrangement is not about folder names. It buys three things:

- **One place per rule.** Password length defaulting existed four times, once
  per provider. The target account defaulting existed twice. Both now live on
  the domain types.
- **A model that outlives the file format.** Adding a YAML key stops at the
  translation layer. Nothing above `internal/infrastructure/configfile` knows
  a YAML file exists.
- **Testable use cases.** The rotation pipeline, including every compensation
  path, is tested against in-memory doubles with no database and no Docker.

## domain

Pure Go, standard library only. No I/O.

**Value objects** carry the invariants that used to be checked far from where
they were used: `SecretName`, `EnvKey`, `FilePath`, `ContainerRef`,
`PasswordLength`, `Schedule`, `Kind`.

`Credential` wraps a secret value and redacts itself in `String`, `GoString`,
every `fmt` verb and `MarshalJSON`. The cleartext is only reachable through
`Expose()`, which makes every place that genuinely needs it greppable: the
database drivers, the `.env` write, and the one deliberate decision to archive
a replaced credential in the audit trail.

**`Secret`** is the aggregate. Its fields are unexported and `NewSecret` is the
only constructor, so an instance that exists is one whose invariants hold; no
code downstream re-checks them.

**`Target`** replaces the untyped option bag: connection details, plus the
defaulting rules for port, target account and account host.

**`Rotation`** is the entity that tracks one attempt. Every mutating step
records what it needs to undo, and `CompensationPlan()` turns that journal into
the ordered list of undo actions. Deciding *what* to roll back is a domain
rule; performing the I/O is not.

**`ports.go`** declares the interfaces the domain needs from the outside world:
`PasswordGenerator`, `CredentialRotator`, `RotatorRegistry`, `EnvStore`,
`EnvDocument`, `ContainerFleet`, `AuditTrail`, `Clock`, `Reporter`. They are
declared next to the code that consumes them, so adding an adapter never
requires touching the core.

## application

Use cases. Ordering and error handling, no rules and no mechanics.

- **`RotateSecret`** drives the pipeline: resolve containers, read the current
  value, generate, apply, verify, write every file, restart. On any failure
  after the first change it runs the domain's compensation plan.
- **`PlanRotation`** answers "what would this do?". Dry run used to be a
  boolean inside the write path, where the two could drift; it is now a
  separate read-only use case returning a value the caller renders.
- **`ScanSecrets`** audits the credentials in a set of files.

## infrastructure

Adapters, one package per concern. Each implements a domain port and declares
it with a compile-time assertion.

| Package | Implements | Notes |
|---------|-----------|-------|
| `credential` | `CredentialRotator`, `PasswordGenerator`, `RotatorRegistry` | one rotator per kind; generation is separate from application |
| `container` | `ContainerFleet` | Docker SDK, Compose service resolution, dependency ordering |
| `envstore` | `EnvStore`, `EnvDocument` | atomic, permission-preserving writes |
| `audit` | `AuditTrail` | AES-256-GCM + Argon2id |
| `configfile` | — | the anti-corruption layer: YAML in, `*domain.Secret` out |
| `notifier` | — | Slack, Discord, generic webhooks |
| `scheduling` | — | driving adapter: decides *when* a use case runs |
| `crypto` | — | encryption primitives used by `audit` |

## interfaces/cli

Cobra commands, and the composition root in `wiring.go`: the single place
where concrete adapters are chosen and handed to the use cases. Swapping the
Docker socket for a socket proxy, or the audit backend for something else, is
a change there and nowhere else.

## Adding a provider

1. Implement `domain.CredentialRotator` in `internal/infrastructure/credential`.
2. Add the kind and its traits to `kinds` in `internal/domain/kind.go`.
3. Register the rotator in `credential.NewRegistry`.

Nothing else changes: validation, defaulting, the pipeline, compensation and
the CLI all work off the kind's traits.
