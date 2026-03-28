# Secret Rotator

Lightweight secret rotation for self-hosted Docker environments.

Auto-discovers secrets in `.env` files, rotates them on schedule or on demand, updates affected containers, and rolls back if anything fails.

## Features

- **Auto-discovery** — scans `.env` files and identifies secrets by naming patterns
- **Password strength audit** — flags weak, default, or short passwords
- **4 providers** — MySQL (ALTER USER), PostgreSQL (ALTER ROLE), Redis (CONFIG SET), Generic
- **Automatic rollback** — LIFO rollback on any failure, restoring DB + .env + containers
- **Encrypted history** — AES-256-GCM + Argon2id encrypted audit log
- **Scheduled rotation** — cron expressions in config or Docker labels
- **Webhook notifications** — Discord, Slack, generic HTTP
- **Zero-config mode** — works without a config file for scanning

## Quick Start

### Binary

```bash
# Download from releases
curl -L https://github.com/GiulioSavini/secret-rotator/releases/latest/download/rotator_linux_amd64.tar.gz | tar xz

# Scan your environment
./rotator scan /path/to/your/project

# Rotate a specific secret
./rotator rotate my-db-password --passphrase "your-master-key"

# Check status
./rotator status
```

### Docker

```yaml
services:
  rotator:
    image: ghcr.io/giuliosavini/secret-rotator:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./:/config
    environment:
      - ROTATOR_MASTER_KEY=your-master-key
    command: daemon
```

## Configuration

See [`rotator.example.yml`](rotator.example.yml) for a full example.

```yaml
secrets:
  - name: my-db-password
    type: mysql
    env_key: MYSQL_PASSWORD
    env_file: .env
    schedule: "0 0 1 * *"  # monthly
    containers:
      - app
      - db
    provider:
      host: db
      port: 3306
      username: root
      password_env: MYSQL_ROOT_PASSWORD
      target_user: app_user
```

### Providers

| Provider | What it does | Config needed |
|----------|-------------|---------------|
| `mysql` | ALTER USER to rotate password | host, port, username, password/password_env |
| `postgres` | ALTER ROLE to rotate password | host, port, username, password/password_env, database |
| `redis` | CONFIG SET requirepass + CONFIG REWRITE | host, port, password/password_env |
| `generic` | Regenerate password + update .env + restart | (none) |

### Docker Labels

Alternative to YAML config for scheduling:

```yaml
labels:
  - "com.secret-rotator.schedule=0 0 1 * *"
  - "com.secret-rotator.my-db-password.schedule=0 0 */7 * *"
```

## Commands

| Command | Description |
|---------|-------------|
| `rotator scan [dir]` | Discover secrets and audit password strength |
| `rotator rotate <name>` | Rotate a specific secret on demand |
| `rotator status` | Show secret states, ages, and schedules |
| `rotator history` | Display encrypted rotation audit log |
| `rotator daemon` | Run scheduled rotations in background |
| `rotator version` | Show version info |

## Security

- Secrets encrypted at rest with AES-256-GCM + Argon2id key derivation
- Docker socket required for container management (use a socket proxy for least-privilege)
- SQL injection protection in all database providers
- Atomic `.env` writes prevent corruption on crash

## License

MIT
