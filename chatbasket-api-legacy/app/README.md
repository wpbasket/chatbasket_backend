# app/
Main entrypoint for the Echo server. Contains `main.go` wiring: env load, DI setup (handlers/services/repo), middleware registration, and route mounting. 

### Timeout Strategy
To support large file uploads (100MB) while maintaining server safety, we use a layered timeout approach:
- **Middleware Safeguard**: Echo middleware provides a 30-second context timeout for all routes EXCEPT upload-related paths.
- **Server Config**: `ReadTimeout` and `WriteTimeout` are set to 600s (10m) to allow data transfer of large payloads without the TCP connection being closed prematurely.

Keep this folder thin—business logic lives in `services/` and db access via sqlc packages.
