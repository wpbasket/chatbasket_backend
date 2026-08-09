package personal_sse

import (
	"context"
	"net/http"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	rpc_personal_ssev1connect "chatbasket-api/gen/proto/personal/personal_sse/rpc_personal_ssev1connect"
	"chatbasket-api/internal/platform/kit"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type personalSseConnectHandler struct {
	rpc_personal_ssev1connect.UnimplementedPersonalSseServiceHandler
	personalSseManager *Manager
}

func newPersonalSseConnectHandler(sm *Manager) rpc_personal_ssev1connect.PersonalSseServiceHandler {
	return &personalSseConnectHandler{
		personalSseManager: sm,
	}
}

func (s *personalSseConnectHandler) StreamEvents(
	ctx context.Context,
	req *connect.Request[rpc_personal_ssev1.PersonalSseEventsRequest],
	stream *connect.ServerStream[rpc_personal_ssev1.PersonalSseEvent],
) error {
	userID, err := kit.GetConnectRpcUserID(ctx)
	if err != nil {
		return kit.ParseIntoRpcError(err)
	}

	sessionID, _ := kit.GetConnectRpcSessionID(ctx)
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	sessionUUID, _ := kit.GetConnectRpcSessionUUID(ctx)
	isPrimary, _ := kit.GetConnectRpcIsPrimary(ctx)

	if s.personalSseManager == nil {
		return kit.ParseIntoRpcError(kit.NewError(http.StatusInternalServerError, "internal_error", "stream manager unavailable"))
	}

	conn, ok := s.personalSseManager.Register(userID.UuidUserId, sessionUUID, isPrimary)
	if !ok {
		return kit.ParseIntoRpcError(kit.NewError(http.StatusTooManyRequests, "rate_limit", "max stream connections reached"))
	}
	defer s.personalSseManager.Unregister(conn)

	// Send initial connection ack event so HTTP response headers are flushed immediately to client
	initEvent := &rpc_personal_ssev1.PersonalSseEvent{
		Timestamp: timestamppb.Now(),
	}
	if err := stream.Send(initEvent); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case msg, open := <-conn.Send:
			if !open {
				return nil
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}
