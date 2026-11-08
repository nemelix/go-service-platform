**English** | [Русский](README.ru.md)

# go-service-platform

[![CI/CD](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml)

A small Go service I use to practice DevOps: containerization, CI/CD, deployment to my own server, HTTPS and observability.

Live: https://64-188-81-197.sslip.io/healthz

## Architecture

```mermaid
flowchart LR
    dev(Developer) -->|Pull Request| checks

    subgraph checks[Checks, in parallel]
        test[go test<br/>with PostgreSQL]
        lint[golangci-lint]
        hado[hadolint]
        sc[shellcheck]
        gl[gitleaks]
    end

    checks --> build[docker build]
    build --> trivy{Trivy<br/>HIGH/CRITICAL?}
    trivy -->|found| stop((stop))
    trivy -->|"clean (main only)"| reg[(ghcr.io<br/>tag = sha)]
    reg --> deploy[Deploy<br/>SSH, forced command]
    deploy -->|docker compose pull| vps
    reg -.->|daily| scan[Scheduled<br/>Trivy scan]
    user(User) -->|HTTPS| nginx

    subgraph vps[VPS]
        nginx[Nginx<br/>TLS, reverse proxy] --> app[Go app]
        app --> db[(PostgreSQL)]
    end
```

## The application

A visit counter stored in PostgreSQL.

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | liveness: the process is alive (never touches the database) |
| `GET /readyz` | readiness: checks the database, answers 503 if it is unavailable |
| `GET /api/visits` | current number of visits |
| `POST /api/visits` | adds a visit and returns the new number |
| `GET /metrics` | Prometheus metrics (closed in Nginx, reachable only inside the Docker network) |

Configuration comes from environment variables ([12-factor](https://12factor.net/config): settings live outside the code), so the same image runs on a laptop, on a VPS and, later, in Kubernetes:

| Variable | Default | Meaning |
|---|---|---|
| `APP_ADDR` | `:8080` | address the HTTP server listens on |
| `DATABASE_URL` | none, required | PostgreSQL connection string; if the database is unreachable, the app exits with an error at startup |

How it behaves:

- **Logs** are structured JSON (`log/slog`) on stdout: one line per event and per request, so a log collector parses fields instead of regexes.
- **Graceful shutdown:** on SIGTERM or SIGINT the server stops accepting new connections, finishes the requests in flight (up to 10 seconds) and exits with code 0.
- **Migrations** are numbered SQL files (`migrations/`, goose) that change the database schema step by step; the database remembers which ones are already applied. They are embedded into the binary and applied at startup, before the server starts listening.
- **Metrics:** `http_requests_total`, `http_request_duration_seconds` (labels: code, method) and `visits_total`. There is no `path` label on purpose: every unique value would create a new time series (a separate stored sequence of numbers), and a client could blow up the monitoring with random URLs.
- **Counter without lost updates:** a visit is added with a single `INSERT ... ON CONFLICT DO UPDATE` command, so concurrent requests do not overwrite each other. An integration test checks that 20 parallel requests give exactly +20.

## Stack and why

- **Go** - builds into a single dependency-free binary, so the image is ~19 MB. Docker, Kubernetes and Terraform are written in Go, and I wanted to get to know the language.
- **PostgreSQL** - the standard relational database, easy to run in a container.
- **pgx** - a native PostgreSQL driver with a connection pool, no ORM.
- **goose** - plain SQL migrations with Up/Down sections, embedded into the binary, so the distroless image needs no extra files.
- **log/slog** - structured logging from the standard library, no extra dependency.
- **Prometheus client** - the de facto standard for metrics.
- **Docker + Compose** - the standard, and Compose is enough for one server. Kubernetes is overkill for now.
- **distroless** - a base image with no shell and no package manager. scratch is too empty (no CA certificates, no users), alpine has extra stuff.
- **GitHub Actions + ghcr.io** - the code is already on GitHub, CI and registry live in one place, and it's free for a public repository. Dependabot keeps the versions of actions and base images up to date.
- **Nginx + certbot** - Caddy or Traefik would issue a certificate on their own, but I wanted to understand ACME and TLS by doing it by hand.
- **sslip.io** - DNS based on the IP address, lets me get a real Let's Encrypt certificate without buying a domain.

Server: VPS with Ubuntu 24.04, 2 vCPU / 4 GB.

## CI/CD

| Event | What happens |
|---|---|
| Pull Request | checks in parallel → image build → Trivy |
| Merge to `main` | the same + publishing the image to ghcr.io → deploy to the server → smoke test |
| Every day (06:30 UTC) | Trivy scans the published image against a fresh vulnerability database |

Checks on every PR:

| Check | What it looks for |
|---|---|
| `go test -race` | broken logic and data races; tests that need a database run against a temporary PostgreSQL that CI starts next to the job |
| golangci-lint | mistakes in Go code, including unchecked errors (`errcheck`) and everything `go vet` finds |
| hadolint | bad practices in the Dockerfile |
| ShellCheck | bugs in shell scripts |
| gitleaks | secrets (tokens, keys) in the whole git history |
| Trivy | HIGH/CRITICAL vulnerabilities in the built image that already have a fix |

- Every image is tagged with the commit sha - the tag tells exactly what code is running, and I can roll back to any version.
- Deploy: CI connects over SSH and passes the sha, a script on the server saves the tag to `.env` and updates only the app container. The compose file, the Nginx config and the secret files are managed by hand on the server for now.
- After the deploy CI checks `/healthz` over HTTPS.
- `main` is protected: changes get in only through a PR where every check passed. Each check is added to the ruleset separately.
- Tests without a database use a fake store behind an interface, so they are fast. Tests with a database are skipped when `DATABASE_URL` is not set and run in CI.

## Security

- **Image:** distroless with no shell and no package manager, runs as non-root.
- **Deploy:** a separate `deploy` user with no password and no sudo. Its SSH key is limited with `restrict` and a forced command - it can only run the deploy script.
- **Deploy script:** owned by root, accepts only a 40-character commit sha.
- **CI:** the server's key is checked via `known_hosts` (no `StrictHostKeyChecking=no`), secrets live in the `production` environment, the token has minimal permissions.
- **Network:** only Nginx is exposed (80/443). The app and PostgreSQL are reachable only inside the Docker network, the database has no published port.
- **Database secrets:** the password lives in `db.env` and `app.env` on the server and never gets into git; the repository holds only `*.env.example` files with placeholders.
- **Errors:** on a failure the API answers with a generic `internal error`; the details go to the logs only.
- **Metrics:** `/metrics` is blocked in Nginx (404), and it will be scraped inside the Docker network.
- **TLS:** TLS 1.2/1.3 only, HTTP to HTTPS redirect, automatic certificate renewal.
- **Vulnerabilities:** Trivy keeps images with fixable HIGH/CRITICAL vulnerabilities out of the registry. Unfixed ones (`ignore-unfixed`) don't block the build: there's nothing to do about them yet, and a CI that is always red stops being read. The published image is also scanned every day.
- **Secrets in code:** gitleaks scans the whole git history on every PR, and GitHub push protection is on - a push containing a token is rejected before it reaches the repository.
- **Third-party actions:** actions are pinned to a commit sha, not a tag: a tag can be moved to other code, a commit can't. GitHub's own actions (`actions/*`) stay on tags: CI trusts GitHub anyway. Dependabot sends sha updates.

## Problems I ran into

- **The server was running an old version even though the deploy went fine.** The server and the registry had different images under the same `latest` tag. I switched to tagging by commit sha - now it's obvious what is running, and I can roll back.
- **A container port was reachable from the internet although ufw blocked it.** Docker publishes ports via DNAT, that traffic goes through the FORWARD chain and never reaches the ufw rules in INPUT. Now only Nginx is published.
- **The container crashed with `exec /server: no such file or directory`, while the file was there.** The binary was built dynamically (without `CGO_ENABLED=0`), and distroless has no libc or loader. I found it with `ldd` on the build stage.
- **The linter found a real bug.** errcheck pointed at the unchecked error from `http.ListenAndServe`: if the port was busy, the app quietly exited with code 0. Now the error is written to the log, and the exit code is 1.
- **A PR could be merged although the linter failed.** A failed `lint` made `build` skipped, and GitHub treats a skipped required check as passed. I made every job a required check.
- **Trivy stopped the image from being published.** A HIGH vulnerability in `crypto/x509` from the Go standard library: I don't call x509 myself, but it's compiled in through `net/http`. I bumped Go to the patch version with the fix.
- **Changing `POSTGRES_PASSWORD` did not change the password.** The postgres image reads this variable only when it initializes an empty data directory. For an existing volume I had to change the password inside the database (`ALTER USER`), update both env files and recreate the containers (`restart` does not re-read `env_file`). A successful `psql` inside the container proves nothing here, because local connections there are trusted; the real check is the app connecting over the network.

Details and more stories: [docs/problems.md](docs/problems.md).

## Known limitations

- `POST /api/visits` is public and has no rate limit. It is a demo endpoint; a limit can be added in Nginx (`limit_req`).
- `visits_total` lives in the process memory and restarts from zero; the real number is in the database. Metrics show how fast things happen and how that changes (visits per minute, is it growing), while the database holds the exact total.
- The compose file, the Nginx config and the env files are copied to the server by hand. Ansible will replace that later.

## Repository layout

```
.
├── main.go                      # startup, configuration, graceful shutdown, logging
├── server.go                    # HTTP routes and handlers
├── store.go                     # visit storage on PostgreSQL
├── db.go                        # connection pool
├── migrate.go                   # goose migrations, embedded into the binary
├── metrics.go                   # Prometheus metrics
├── migrations/                  # SQL migrations
├── main_test.go, server_test.go # unit tests (fake store)
├── store_test.go                # integration tests (real PostgreSQL)
├── Dockerfile                   # multi-stage build
├── compose.dev.yml              # PostgreSQL for local tests
├── docs/
│   ├── problems.md              # problems I ran into, in detail
│   └── problems.ru.md           # the same in Russian
├── .github/
│   ├── workflows/
│   │   ├── ci.yml               # CI/CD: checks → build + Trivy → deploy
│   │   └── scheduled-scan.yml   # daily image scan
│   └── dependabot.yml           # automatic dependency updates
└── deploy/
    ├── docker-compose.yml       # app + PostgreSQL + Nginx + certbot
    ├── nginx/default.conf       # reverse proxy, TLS
    ├── deploy.sh                # deploy script on the server
    ├── db.env.example           # template for the database password
    └── app.env.example          # template for DATABASE_URL
```

## Run locally

The app needs PostgreSQL. A throwaway one for development is in `compose.dev.yml` (port 5432 on `127.0.0.1` only, password `dev`):

```bash
docker compose -f compose.dev.yml up -d

export DATABASE_URL='postgres://app:dev@localhost:5432/app?sslmode=disable'
go run .                        # migrations are applied on startup

curl localhost:8080/healthz
curl -X POST localhost:8080/api/visits
curl localhost:8080/api/visits
curl localhost:8080/metrics
```

## Tests

Without `DATABASE_URL` the tests that need a database are skipped:

```bash
go test ./...
```

With the development database (`-count=1` disables result caching).

Linux / macOS:

```bash
DATABASE_URL='postgres://app:dev@localhost:5432/app?sslmode=disable' go test -v -count=1 ./...
```

> [!IMPORTANT]
> **You must install [Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/) first.**

Windows (PowerShell). The variable stays set in the current terminal until you close it or run `Remove-Item Env:DATABASE_URL`:

```powershell
$env:DATABASE_URL = 'postgres://app:dev@localhost:5432/app?sslmode=disable'
go test -v -count=1 ./...
```

### In Docker

The app container must see the database container, so both run in the same Docker network. The network (`go-service-platform-dev_default`) is created when you start the database from `compose.dev.yml`, and inside it the database is reachable by the name `db`. The commands are the same on Linux, macOS and Windows (PowerShell):

```bash
docker compose -f compose.dev.yml up -d
docker build -t go-service-platform:dev .
docker run --rm -p 8080:8080 --network go-service-platform-dev_default -e DATABASE_URL="postgres://app:dev@db:5432/app?sslmode=disable" go-service-platform:dev
```

Stop it with Ctrl+C. Check from a second terminal: `curl localhost:8080/readyz` (in PowerShell use `curl.exe`).

## First-time server setup (manual for now, before Ansible)

You need a server with Docker and Compose. A user `deploy` (no password, no sudo) owns the app directory and runs `docker compose`. On the server:

```bash
sudo adduser --disabled-password --gecos "" deploy
sudo usermod -aG docker deploy
sudo mkdir -p /opt/go-service-platform
sudo chown deploy:deploy /opt/go-service-platform
```

Membership in the `docker` group is effectively root, so this user's SSH key is restricted: it can only run the deploy script (`restrict` and a forced command). The `docker-compose.yml` and `nginx/default.conf` files from the repository's `deploy/` directory are copied to `/opt/go-service-platform` by hand.

Check that everything is in place:

```bash
id deploy && ls -ld /opt/go-service-platform
```

### Secrets and start

The secret files are created once by hand:

```bash
cd /opt/go-service-platform

# db.env: generate a password (24 bytes as hex)
echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)" | sudo tee db.env > /dev/null

# app.env: the connection string is built from the password in db.env
sed 's|^POSTGRES_PASSWORD=\(.*\)|DATABASE_URL=postgres://app:\1@db:5432/app?sslmode=disable|' db.env | sudo tee app.env > /dev/null

# owner and permissions: only deploy can read them
sudo chown deploy:deploy db.env app.env
sudo chmod 600 db.env app.env

# start the stack
docker compose up -d
docker compose ps            # db must be healthy
```

## Plans

- [ ] Observability (metrics, dashboards, alerts, logs): Prometheus / VictoriaMetrics, Grafana, alerts
- [ ] Backups: `pg_dump` to restic (a backup tool) with a tested restore
- [ ] Ansible and OpenTofu - server setup and infrastructure as code
- [ ] Kubernetes (k3d), Helm, ArgoCD