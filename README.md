# Chatbasket Backend

## Overview

Chatbasket Backend is a Go-based HTTP API built with the Echo framework. It provides backend services for the Chatbasket application, including:

- User and personal contact management
- Public and personal routes
- PostgreSQL-backed persistence
- Health checks and basic observability
- **Fully Automated Deployment Pipeline**

## Tech Stack

- **Language:** Go (module: `chatbasket`)
- **Framework:** Echo v4
- **Database:** PostgreSQL (via `pgx` connection pool)
- **Env Management:** `github.com/joho/godotenv`
- **Infrastructure:** Docker, Nginx, DigitalOcean, GitHub Actions

## 🚀 Automated Deployment

This repository features a **zero-touch deployment pipeline**.

### How it Works
1. Push to `main` branch.
2. GitHub Actions builds the Docker image and pushes to GHCR.
3. Workflow SSHs into DigitalOcean droplet and:
   - Updates `docker-compose.yml` and `nginx.conf`
   - Generates `.env` from GitHub Secrets
   - Pulls new images and restarts containers

### Setup & Documentation
Refactored deployment documentation is available in the `docs/` folder:

- **[GitHub Secrets Setup](docs/GITHUB_SECRETS_SETUP.md)** - Required configuration for the pipeline
- **[Deployment Verification](docs/DEPLOYMENT_VERIFICATION.md)** - How to verify deployments are working
- **[Nginx Configuration](docs/nginxconf.md)** - Reverse proxy setup details
- **[.env.example](docs/.env.example)** - Template for environment variables

### Manual Maintenance
The only manual steps required are initial droplet setup or one-time secret updates:
- **Reseved IP**: Ensure your Azure PostgreSQL Firewall allows the droplet's **Anchor IP** (check via `curl ifconfig.me`).
- **Secret Rotation**: Update secrets in GitHub and push an empty commit to redeploy.

## Project Structure

- **`chatbasket-api/`** – Application source code (`main.go`, `db/`, `routes/`, etc.)
- **`deployment/`** – Infrastructure configuration (`docker-compose.yml`, `nginx.conf`)
- **`docs/`** – Deployment and setup documentation
- **`.github/workflows/`** – CI/CD Pipeline definitions

## Development

### Running Locally

```bash
cd chatbasket-api
go mod tidy
go run ./app
```

### Running with Docker

```bash
cd chatbasket-api
docker build -t chatbasket-api .
docker run -p 8080:8080 --env-file ../.env chatbasket-api
```

## CORS and Frontend
CORS allows:
- `https://chatbasket.live` (Production)
- `http://localhost:8081` (Local dev - uncomment in `main.go` if needed)

## Graceful Shutdown & Health Checks

The server supports production-friendly behavior:

- Graceful shutdown on `SIGTERM` / interrupt
- Connection pool cleanup with timeouts
- Health check at `/healthz` that pings PostgreSQL with a short timeout

## Development Notes

- Update or extend routes in `routes/` and corresponding handlers/services.
- Schema or query changes should be reflected in `db/` and any generated code.
- Keep `.env` out of version control and use environment variables for secrets in production.
