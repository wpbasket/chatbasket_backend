package personal_sse

import (
	"context"
	"net/http"

	rpc_personal_ssev1connect "chatbasket-api/gen/proto/personal/personal_sse/rpc_personal_ssev1connect"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
)

// Register initializes the Personal SSE module: instantiates its manager and starts the Postgres listener.
func Register(pool *pgxpool.Pool) *Manager {
	personalSseManager := NewManager(pool)
	go StartPostgresListener(context.Background(), pool, personalSseManager)
	return personalSseManager
}

// RegisterRoutes registers the Personal SSE ConnectRPC routes on the given group.
func RegisterRoutes(personalGroup *echo.Group, manager *Manager) {
	connectHandler := newPersonalSseConnectHandler(manager)
	path, handler := rpc_personal_ssev1connect.NewPersonalSseServiceHandler(connectHandler)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", handler)))
}
