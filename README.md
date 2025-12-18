# Chatbasket Backend

## Overview

Chatbasket Backend is a Go-based HTTP API built with the Echo framework. It provides backend services for the Chatbasket application, including:

- User and personal contact management
- Public and personal routes
- PostgreSQL-backed persistence
- Health checks and basic observability

The application is container-ready and can be built and run using Docker.

## Tech Stack

- **Language:** Go (module: `chatbasket`)
- **Framework:** Echo v4
- **Database:** PostgreSQL (via `pgx` connection pool)
- **Env Management:** `github.com/joho/godotenv`
- **Other:** Appwrite Go SDK, CORS, gzip, rate limiting middleware

## Project Structure

Key directories inside `chatbasket/`:

- **`app/`** – Application entrypoint (`main.go`)
- **`db/`** – Database configuration and queries
- **`model/` / `personalModel/`** – Data models
- **`routes/`** – Route registration
- **`services/`, `personalServices/`, `publicServices/`** – Business logic
- **`handler/`, `personalHandler/`, `publicHandler/`** – HTTP handlers
- **`middleware/`** – Custom middleware
- **`utils/`, `personalUtils/`** – Helper utilities
- **`Dockerfile`** – Multi-stage Docker build for the API

## Requirements

- Go (compatible with version in `go.mod` – `go 1.25.5`)
- PostgreSQL instance
- Appwrite (if using Appwrite integrations)
- Git
- Docker (optional, for containerized runs)

## Environment Configuration

Environment variables are loaded from `.env` at the project root (relative to `app/main.go` it uses `../.env`). Typical variables include:

- **Database:** `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- **Server:** `PORT` (defaults to `8080` if not set)
- **Appwrite / Auth / Other:** e.g. API keys, endpoint URLs, project IDs, secrets, etc.

> Configure the `.env` file with the values required by your deployment (database, Appwrite, auth config, etc.). Do **not** commit secrets to version control.

## Running Locally (Go)

From the `chatbasket/` directory:

```bash
# Install dependencies (Go modules will auto-resolve)
go mod tidy

# Run the server
go run ./app
```

The API will start on the port defined by `PORT` in `.env`, or on `:8080` by default.

Health check endpoint:

- `GET /healthz` – returns an `ok`/`unhealthy` JSON status depending on DB health

## Running with Docker

From the `chatbasket/` directory:

```bash
# Build the image
docker build -t chatbasket-backend .

# Run the container (example; adjust envs/ports as needed)
docker run \
  -p 8080:8080 \
  --env-file ../.env \
  --name chatbasket-backend \
  chatbasket-backend
```

The container runs the compiled Go binary from `./main` built in the Dockerfile.

## CORS and Frontend

CORS is configured in `app/main.go`. By default it allows origins such as:

- `http://localhost:8081` (local frontend)

You can update the `AllowOrigins` list in `main.go` to add or change allowed frontend URLs (e.g. production domain).

## Graceful Shutdown & Health Checks

The server supports production-friendly behavior:

- Graceful shutdown on `SIGTERM` / interrupt
- Connection pool cleanup with timeouts
- Health check at `/healthz` that pings PostgreSQL with a short timeout

## Development Notes

- Update or extend routes in `routes/` and corresponding handlers/services.
- Schema or query changes should be reflected in `db/` and any generated code.
- Keep `.env` out of version control and use environment variables for secrets in production.
