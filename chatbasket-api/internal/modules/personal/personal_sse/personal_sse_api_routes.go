package personal_sse

import (
	"context"
	"net/http"

	rpc_personal_ssev1connect "chatbasket-api/gen/proto/personal/personal_sse/rpc_personal_ssev1connect"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// Register initializes the Personal SSE module: instantiates its manager and starts the Postgres listener.
func Register(pool *pgxpool.Pool) *Manager {
	personalSseManager := NewManager(pool)
	go StartPostgresListener(context.Background(), pool, personalSseManager)
	return personalSseManager
}

// RegisterRoutes adds the live event stream RPC to the server.
// The stream stays open for a long time, so it has NO time limit.
// It still has a 5MB upload limit, because the request body is tiny.
func RegisterRoutes(personalGroup *echo.Group, manager *Manager) {
	connectHandler := newPersonalSseConnectHandler(manager)
	path, handler := rpc_personal_ssev1connect.NewPersonalSseServiceHandler(connectHandler)
	// Stay-open stream: no timeout by design. Importing our own middleware
	// package here would create an import cycle (it already imports this
	// package for auth sessions), so we use Echo's BodyLimit directly.
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", handler)),
		middleware.BodyLimit(5242880),
	)
}
