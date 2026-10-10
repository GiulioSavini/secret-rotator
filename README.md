# Secret Rotator

Secret rotation for self-hosted Docker environments.

Discovers secrets in `.env` files, rotates them on schedule, on policy or on
demand, updates the backing service, restarts the affected containers in
dependency order, and rolls back if anything fails.

Rotation policies are expressed as a maximum age ("no credential older than 90
days"), and `rotator status --overdue` turns that into a monitoring check.

## Quick start

```bash
# 1. Get the binary (linux_arm64, darwin_amd64 and darwin_arm64 are also published)
curl -L https://github.com/GiulioSavini/secret-rotator/releases/latest/download/rotator_linux_amd64.tar.gz | tar xz

# ...or build it yourself
go install github.com/GiulioSavini/secret-rotator/cmd/rotator@latest

# 2. See what you have
cd /path/to/your/project
./rotator scan

# 3. Generate a config from it
./rotator init

# 4. Fill in the TODOs it prints, then check the plan
export ROTATOR_MASTER_KEY="$(openssl rand -base64 32)"
./rotator status
./rotator rotate mysql_root_password --dry-run

# 5. Rotate
./rotator rotate mysql_root_password
```

`rotator init` reads your `.env` files and the containers running on the
daemon, and writes a `rotator.yml` with everything it could infer. It prints
the fields it could not, and never writes a secret value into the file.

### Docker

Add [`docker-compose.yml`](docker-compose.yml)'s `rotator` service to your
project, then:

```bash
echo "ROTATOR_MASTER_KEY=$(openssl rand -base64 32)" >> .env
echo "DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)" >> .env
docker compose run --rm rotator init
# edit rotator.yml
docker compose up -d rotator
```

The image mounts your project at `/config`, which is both its working
directory and the first place it looks for `rotator.yml`. The encrypted
history lives on a separate `/data` volume.

Two things the image needs that are easy to miss:

- **`/config` must be writable.** Rotation rewrites your `.env` files in
  place. The container runs as uid `65532`, so that uid needs write access to
  the project directory and the files in it.
- **Access to the Docker socket.** `group_add: ["${DOCKER_GID}"]` is what
  grants the nonroot user access to the mounted socket.

There is a complete, runnable stack under
[`examples/quickstart/`](examples/quickstart/).

## Configuration

Discovered automatically, in this order:

1. `--config <path>`
2. `$ROTATOR_CONFIG`
3. `./rotator.yml`, `./rotator.yaml`, `./.rotator.yml`
4. `/config/rotator.yml`, `/config/rotator.yaml`
5. `/etc/rotator/rotator.yml`, `/etc/rotator/rotator.yaml`

Relative paths inside the file are resolved against the file itself, so a
scheduled run and a manual run from another directory act on the same `.env`.

See [`rotator.example.yml`](rotator.example.yml) for a fully commented
example.

```yaml
master_key_env: ROTATOR_MASTER_KEY

secrets:
  - name: mysql_root_password
    type: mysql
    env_key: MYSQL_ROOT_PASSWORD
    env_file: .env
    containers: [db, app]
    provider:
      host: db
      port: "3306"
      username: root
      password_env: MYSQL_ROOT_PASSWORD
    schedule: "0 3 1 * *"
    length: 32
```

### Providers

| `type` | What it does | Required under `provider:` |
|--------|--------------|----------------------------|
| `mysql` | `ALTER USER` on `target_user` (default: `username`) | `host`, `username`, and `password` or `password_env` |
| `postgres` | `ALTER ROLE` on `target_user` (default: `username`) | `host`, `username`, `database`, and `password` or `password_env` |
| `redis` | `CONFIG SET requirepass` + `CONFIG REWRITE` | `host` |
| `generic` | New value in the `.env` files, then restart | — |
| `command` | Runs programs you name | a `command:` block (see below) |

Common optional keys: `port`, `target_user`, `target_user_host` (MySQL's
`'user'@'host'`, default `%`), `length`. An unrecognised key under `provider:`
is reported as a warning rather than ignored, so a typo does not silently
rotate the wrong account.

`password_env` names an environment variable holding the **current** admin
password. It is looked up in the secret's `.env` file first and in rotator's
own environment second. Reading the file first is what makes repeated
rotations work when the admin credential is itself the secret being rotated:
a long-running daemon keeps the environment it started with.

### The `command` provider

For anything without a built-in provider — MinIO, RabbitMQ, Keycloak, Grafana,
SSH `authorized_keys`, an HTTP token endpoint — name the programs that do the
work:

```yaml
secrets:
  - name: minio_secret_key
    type: command
    env_key: MINIO_SECRET_KEY
    env_file: .env
    command:
      apply:  ["./hooks/rotate-minio.sh", "apply"]
      verify: ["./hooks/rotate-minio.sh", "verify"]
      timeout: 60s
      pass_env: [MINIO_ALIAS]
    max_age: 90d
```

Only `apply` is required. Without `verify` the new value is written to the
`.env` files without being proven to work. Without `restore`, a failed
rotation is undone by running `apply` again with the previous value, which is
right whenever applying is idempotent.

Your program receives everything in its environment:

| Variable | Meaning |
|----------|---------|
| `ROTATOR_OPERATION` | `apply`, `verify` or `restore` |
| `ROTATOR_SECRET_NAME`, `ROTATOR_ENV_KEY` | which secret this is |
| `ROTATOR_NEW_SECRET` | the value to set, or to check |
| `ROTATOR_PREVIOUS_SECRET` | the value to put back (`restore`) |
| `ROTATOR_CURRENT_SECRET` | what the service accepts right now (`restore`) |
| `ROTATOR_HOST`, `ROTATOR_PORT`, `ROTATOR_USERNAME`, `ROTATOR_TARGET_USER`, `ROTATOR_DATABASE` | the target, when configured |

Exit `0` for success, non-zero for failure. There is a working example in
[`examples/command-provider/`](examples/command-provider/).

**What this provider will and will not do**, because it runs code your config
file chose:

- **argv lists, never shell strings.** There is no shell, so there is nothing
  to inject into and no quoting to get wrong. A single string containing
  spaces is rejected with an explanation rather than handed to `/bin/sh`.
- **No credential in the arguments.** `/proc/<pid>/cmdline` is world-readable
  and `ps` would show it to every user on the host, so values travel in the
  environment, which only the same uid and root can read.
- **rotator's environment is not inherited.** It holds `ROTATOR_MASTER_KEY`;
  passing that to every hook would make the passphrase readable by anything
  the script runs. The child gets `PATH`, `HOME`, the `ROTATOR_*` context, and
  whatever `pass_env` names explicitly.
- **Every invocation is bounded**, and the timeout covers the whole process
  tree, so a script that hangs or leaves a child behind cannot wedge the
  daemon.
- **Output is captured and quoted back in errors**, truncated at 2 KB. Do not
  echo the secret: it would end up in the audit trail.
- Relative program paths and `working_dir` resolve against `rotator.yml`, the
  same rule the `.env` paths follow.

### Containers

`containers:` accepts either real container names or Compose **service**
names — rotator maps service names onto the containers Docker actually
created (`<project>-<service>-<n>`) via the `com.docker.compose.service`
label, expanding scaled services into all their replicas.

Restart order comes from `depends_on` in your Compose file, which is
auto-discovered next to `rotator.yml` (override with `compose_file:`). Without
one, the order you wrote in `containers:` is used as-is.

### When rotation happens

Two independent triggers. A secret can use either, or both.

**A schedule** says *when* to rotate — a cron expression in
`secrets[].schedule`, or on a container label:

```yaml
labels:
  - "com.secret-rotator.schedule=0 0 1 * *"
  - "com.secret-rotator.mysql_root_password.schedule=0 0 */7 * *"
```

**A policy** says what must be *true* — `max_age`, in `d`, `w`, `h`, `m` or
`s`:

```yaml
secrets:
  - name: mysql_root_password
    max_age: 90d        # no credential older than 90 days
```

The difference matters when the daemon is not running on the day the cron
expression would have fired: a schedule silently skips that month, a policy
notices and rotates on the next check (hourly by default,
`ROTATOR_POLICY_INTERVAL` to change it). A secret with only `max_age` and no
schedule is a complete configuration.

`rotator daemon` runs both in the foreground and stops on SIGINT/SIGTERM.
Rotations of the same secret never overlap, whichever trigger fired.

### Monitoring the policy

`rotator status` shows the age of each credential, the policy it has to meet,
and whether it meets it. `--overdue` narrows it to what needs attention and
exits with a dedicated code, so it drops straight into monitoring or a CI
gate:

```bash
rotator status --overdue    # 0 = all within policy, 2 = something is not
```

| Exit code | Meaning |
|-----------|---------|
| `0` | every secret is within its `max_age` |
| `1` | the command failed (bad config, unreachable service, ...) |
| `2` | at least one secret needs attention |

Ages come from the encrypted rotation history, so evaluating a policy needs
`ROTATOR_MASTER_KEY`. Without it the state is reported as `unknown` and the
check still exits non-zero: a policy that cannot be evaluated is not a policy
that has been met.

## Commands

| Command | Description |
|---------|-------------|
| `rotator init [dir]` | Generate a `rotator.yml` from `.env` files and running containers |
| `rotator scan [dir]` | Discover secrets and audit password strength |
| `rotator rotate <name>` | Rotate one secret on demand |
| `rotator status` | Ages, policies, compliance and next rotation times |
| `rotator history` | Decrypt and show the rotation audit log |
| `rotator daemon` | Run scheduled rotations in the foreground |
| `rotator version` | Version information |

Global flags: `--config`, `--data-dir`, `--dry-run`, `--verbose`.

Environment: `ROTATOR_CONFIG`, `ROTATOR_DATA_DIR`, `ROTATOR_MASTER_KEY`,
`ROTATOR_POLICY_INTERVAL`, `ROTATOR_AUDIT_OMIT_PREVIOUS`.

## Security

- History entries are encrypted with AES-256-GCM using an Argon2id-derived
  key, and the file is written `0600`.
- `.env` files are rewritten atomically (temp file, `fsync`, rename) and keep
  their original permissions.
- Generated passwords are URL-safe base64 from `crypto/rand`; `length` is the
  number of random bytes, so `length: 32` yields a 43-character password.
- The Docker socket is mounted read-only, but read access to it is still
  equivalent to root on the host. For least privilege, put a socket proxy in
  front of it and allow only `GET /containers` plus
  `POST /containers/*/restart`.

## Known limitations

Real, reproducible gaps, listed here rather than left to be discovered in
production.

- **The history keeps the previous secret in cleartext** inside the encrypted
  entry, indefinitely. Treat `history.json` as a credential store and control
  access to it accordingly. Set `ROTATOR_AUDIT_OMIT_PREVIOUS=1` to stop
  recording it, at the cost of losing the ability to recover a replaced value.
- **No lock between instances.** The daemon serialises rotations of the same
  secret within one process; two rotator processes over the same project are
  not coordinated.
- **Redis needs a rewritable config file.** Rotation uses `CONFIG SET
  requirepass` followed by `CONFIG REWRITE`. A server started with
  `--requirepass` on the command line has nothing to rewrite; the rotation
  reports this and rolls itself back rather than leaving the new password in
  memory only.
- **No metrics or health endpoint.** The daemon logs to stderr; there is
  nothing to scrape and nothing for a liveness probe to hit.

## Architecture

Hexagonal, with a domain-driven core: `internal/domain` holds the model and the
ports, `internal/application` the use cases, `internal/infrastructure` the
adapters, `internal/interfaces/cli` the commands and the composition root.
`internal/architecture_test.go` fails the build if a dependency points the
wrong way.

See [ARCHITECTURE.md](ARCHITECTURE.md), including how to add a provider.

## Development

```bash
make build   # ./bin/rotator
make test    # go test ./...
make lint    # go vet ./...
make docker  # build the image
```

## License

MIT
