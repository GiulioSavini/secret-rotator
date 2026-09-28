# Quickstart stack

A throwaway Postgres + Redis + app stack, so you can watch a rotation happen
end to end before pointing rotator at anything you care about.

## Run it

```bash
cd examples/quickstart
cp .env.example .env
echo "ROTATOR_MASTER_KEY=$(openssl rand -base64 32)" >> .env
echo "DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)" >> .env

docker compose up -d db cache app
```

Look at what rotator sees:

```bash
docker compose run --rm rotator scan
docker compose run --rm rotator status
```

`scan` flags both passwords as weak — `.env.example` ships `changeme` on
purpose.

## Rotate

Check the plan first; `--dry-run` touches nothing:

```bash
docker compose run --rm rotator rotate postgres_password --dry-run
```

Then do it:

```bash
grep POSTGRES_PASSWORD .env          # before
docker compose run --rm rotator rotate postgres_password
grep POSTGRES_PASSWORD .env          # after
docker compose run --rm rotator history
```

`db` and `app` are restarted in that order, taken from `depends_on` in
`docker-compose.yml`.

## Run it on a schedule

`rotator.yml` schedules `postgres_password` monthly. To let the daemon drive
it:

```bash
docker compose up -d rotator
docker compose logs -f rotator
```

## Clean up

```bash
docker compose down -v
rm .env
```

## What this example does not cover

`redis_password` is configured but deliberately left without a schedule.
Redis rotation uses `CONFIG SET requirepass` followed by `CONFIG REWRITE`, and
`CONFIG REWRITE` fails unless the server was started from a config file it can
write back to. This stack starts Redis with `--requirepass` on the command
line, so the rewrite would fail and the rotation would roll itself back. To
rotate Redis for real, start it with `redis-server /etc/redis/redis.conf` and
mount a writable config file.
