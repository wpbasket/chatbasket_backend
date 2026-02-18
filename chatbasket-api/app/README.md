# app/
Main entrypoint for the Echo server. Contains `main.go` wiring: env load, DI setup (handlers/services/repo), middleware registration, and route mounting. Keep this folder thin—business logic lives in `services/` and db access via sqlc packages.
