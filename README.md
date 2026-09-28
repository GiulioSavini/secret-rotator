# Secret Rotator

Secret rotation for self-hosted Docker environments.

Discovers secrets in `.env` files, rotates them on schedule or on demand,
updates the backing service, restarts the affected containers in dependency
order, and rolls back if anything fails.

## Quick start

```bash
# 1. Get the binary
curl -L https://github.com/GiulioSavini/secret-rotator/releases/latest/download/rotator_linux_amd64.tar.gz | tar xz

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

Common optional keys: `port`, `target_user`, `length`.

`password_env` names an environment variable holding the **current** admin
password. It is looked up in the secret's `.env` file first and in rotator's
own environment second. Reading the file first is what makes repeated
rotations work when the admin credential is itself the secret being rotated:
a long-running daemon keeps the environment it started with.

### Containers

`containers:` accepts either real container names or Compose **service**
names — rotator maps service names onto the containers Docker actually
created (`<project>-<service>-<n>`) via the `com.docker.compose.service`
label, expanding scaled services into all their replicas.

Restart order comes from `depends_on` in your Compose file, which is
auto-discovered next to `rotator.yml` (override with `compose_file:`). Without
one, the order you wrote in `containers:` is used as-is.

### Scheduling

Cron expressions in `secrets[].schedule`, or on container labels:

```yaml
labels:
  - "com.secret-rotator.schedule=0 0 1 * *"
  - "com.secret-rotator.mysql_root_password.schedule=0 0 */7 * *"
```

`rotator daemon` runs them in the foreground and stops on SIGINT/SIGTERM.

## Commands

| Command | Description |
|---------|-------------|
| `rotator init [dir]` | Generate a `rotator.yml` from `.env` files and running containers |
| `rotator scan [dir]` | Discover secrets and audit password strength |
| `rotator rotate <name>` | Rotate one secret on demand |
| `rotator status` | Secret states, ages and next rotation times |
| `rotator history` | Decrypt and show the rotation audit log |
| `rotator daemon` | Run scheduled rotations in the foreground |
| `rotator version` | Version information |

Global flags: `--config`, `--data-dir`, `--dry-run`, `--verbose`.

Environment: `ROTATOR_CONFIG`, `ROTATOR_DATA_DIR`, `ROTATOR_MASTER_KEY`.

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

These are real, reproducible gaps. They are listed here rather than left to be
discovered in production.

- **Rollback restores only the first `.env` file.** When a secret uses
  `env_files:` with more than one entry, a failed rotation restores the first
  one; the others keep the new value while the database has been rolled back
  to the old one. Use a single `env_file:` per secret until this is fixed.
- **Rollback writes `.env` with mode `0644`.** The normal write path preserves
  the original permissions; the rollback path does not, so a file that was
  `0600` becomes world-readable after a failed rotation.
- **MySQL admin passwords are not escaped in the DSN.** An admin password
  containing `@` or `/` breaks the connection string. Generated passwords are
  unaffected (base64 has no such characters); a hand-set admin password may
  be. PostgreSQL is unaffected.
- **MySQL rotates `'<user>'@'%'` only.** An account defined as
  `'app'@'localhost'` is not matched.
- **The history keeps the previous secret in cleartext** inside the encrypted
  entry, indefinitely. Treat `history.json` as a credential store and control
  access to it accordingly.
- **No lock between instances.** The daemon serialises rotations within one
  process; two rotator processes over the same project are not coordinated.

## Development

```bash
make build   # ./bin/rotator
make test    # go test ./...
make lint    # go vet ./...
make docker  # build the image
```

## License

MIT
