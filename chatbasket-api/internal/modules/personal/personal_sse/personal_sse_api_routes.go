package personal_sse

import (
	"context"
	"net/http"

	rpc_personal_ssev1connect "chatbasket-api/gen/proto/personal/personal_sse/rpc_personal_ssev1connect"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
)

// Register initializes the Personal SSE module, instantiates its manager, starts Postgres listener, and registers ConnectRPC routes.
func Register(personalGroup *echo.Group, pool *pgxpool.Pool) *Manager {
	personalSseManager := NewManager(pool)
	go StartPostgresListener(context.Background(), pool, personalSseManager)

	connectHandler := newPersonalSseConnectHandler(personalSseManager)
	path, handler := rpc_personal_ssev1connect.NewPersonalSseServiceHandler(connectHandler)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", handler)))
	return personalSseManager
}
