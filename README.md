**English** | [Русский](README.ru.md)

# go-service-platform

[![CI/CD](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/nemelix/go-service-platform/actions/workflows/ci.yml)

A small Go service I use to practice DevOps: containers, CI/CD, deploying to my own server and HTTPS.

Live: https://64-188-81-197.sslip.io/health

## Architecture

```mermaid
flowchart LR
dev[Developer] -->|Pull Request| gh[GitHub]
gh --> ci[GitHub Actions<br/>vet · test · build]
ci -->|image tagged with sha| reg[(ghcr.io)]
ci -->|SSH, forced command| vps
reg -->|docker compose pull| vps
user[User] -->|HTTPS| nginx

subgraph vps[VPS]
nginx[Nginx<br/>TLS, reverse proxy] --> app[Go app]
end
```

## Stack and why

- **Go** - builds into a single binary with no dependencies, so the image is ~19 MB. Docker, Kubernetes and Terraform are written in Go, and I wanted to get to know the language.
- **Docker + Compose** - the standard, and Compose is enough for one server. Kubernetes would be overkill for now.
- **distroless** - a base image with no shell and no package manager. scratch is too empty (no CA certificates, no users), alpine has extra stuff I don't need.
- **GitHub Actions + ghcr.io** - the code is already on GitHub, CI and registry live in one place, and it's free for public repos. Dependabot keeps actions and base images up to date.
- **Nginx + certbot** - Caddy or Traefik would get a certificate on their own, but I wanted to understand ACME and TLS by doing it by hand.
- **sslip.io** - DNS based on the IP address, lets me get a real Let's Encrypt certificate without buying a domain.

Server: VPS with Ubuntu 24.04, 2 vCPU / 4 GB.

## CI/CD

| Event | What happens |
|---|---|
| Pull Request | `go vet` → tests → image build (checks that the Dockerfile works) |
| Merge to `main` | the same + push image to ghcr.io → deploy to the server → smoke test |

- Every image is tagged with the commit sha - the tag tells exactly what code is running, and I can roll back to any version.
- Deploy: CI connects over SSH and passes the sha, a script on the server saves the tag to `.env` and updates only the app container.
- After the deploy CI checks `/health` over HTTPS.
- `main` is protected: changes get in only through a PR with green checks.

## Security

- **Image:** distroless with no shell and no package manager, runs as non-root.
- **Deploy:** a separate `deploy` user with no password and no sudo. Its SSH key is limited with `restrict` and a forced command - the only thing it can do is run the deploy script.
- **Deploy script:** owned by root, accepts only a 40-character commit sha.
- **CI:** the server's host key is checked via `known_hosts` (no `StrictHostKeyChecking=no`), secrets live in the `production` environment, the token has minimal permissions.
- **Network:** only Nginx is exposed (80/443), the app is reachable only inside the Docker network.
- **TLS:** TLS 1.2/1.3 only, HTTP to HTTPS redirect, automatic certificate renewal.

## Problems I ran into

- **The server was running an old version even though the deploy went fine.** The server and the registry had different images under the same `latest` tag. I switched to tagging by commit sha - now it's obvious what's running, and rollback is easy.
- **A container port was reachable from the internet although ufw blocked it.** Docker publishes ports via DNAT, that traffic goes through the FORWARD chain and never hits ufw rules in INPUT. Now only Nginx is published.
- **SSH password login was on, although `sshd_config` said `no`.** `sshd_config.d/50-cloud-init.conf` is included earlier, and sshd uses the first value it finds. Now I check the effective settings with `sshd -T`.
- **The container crashed with `exec /server: no such file or directory`, while the file was there.** The binary was built dynamically (without `CGO_ENABLED=0`), and distroless has no libc or dynamic loader. Found it with `ldd` on the build stage.
- **Old images were not removed after deploys.** `docker image prune` only touches untagged images, and mine all have a sha tag. Now I clean them up by the label from the Dockerfile.
- **I didn't want to give GitHub full access to my server.** Hence a separate deploy user and a key that can run only one script.

## Repository layout

```
.
├── main.go                      # the app
├── main_test.go                 # tests
├── Dockerfile                   # multi-stage build
├── .github/
│   ├── workflows/ci.yml         # CI/CD: test → build → deploy
│   └── dependabot.yml           # dependency updates
└── deploy/
    ├── docker-compose.yml       # app + Nginx + certbot
    ├── nginx/default.conf       # reverse proxy, TLS
    └── deploy.sh                # deploy script on the server
```

## Run locally

```bash
# without Docker
go run .
curl localhost:8080/health

# in Docker
docker build -t go-service-platform:dev .
docker run --rm -p 8080:8080 go-service-platform:dev
```

## Plans

- [ ] Terraform and Ansible - server setup as code
- [ ] Kubernetes (k3d), Helm, ArgoCD
- [ ] Monitoring: Prometheus / VictoriaMetrics