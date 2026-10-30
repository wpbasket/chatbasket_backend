package personal_sse

import (
	"context"
	"testing"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"
	"chatbasket-api/internal/platform/connect_sse"
	"chatbasket-api/internal/platform/kit"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestStreamEvents_RateLimitGenericError(t *testing.T) {
	mgr := NewManager(nil)
	handler := newPersonalSseConnectHandler(mgr)

	userID := uuid.New()

	// 1. Fill up 5 sessions for user
	for i := 0; i < 5; i++ {
		sessID := uuid.New()
		connID := uuid.New()
		_, ok := mgr.Register(userID, sessID, connID, false)
		assert.True(t, ok, "session %d registration should succeed", i+1)
	}

	// 6th session for the same user: should fail with generic max stream connections error
	sess6 := uuid.New()
	conn6 := uuid.New()

	ctx6 := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UUIDUserID:  userID,
		SessionUUID: sess6,
		IsPrimary:   false,
	})

	req6 := connect.NewRequest(&rpc_personal_ssev1.PersonalSseEventsRequest{})
	req6.Header().Set(connect_sse.SSEConnectionIDHeader, conn6.String())

	err6 := handler.StreamEvents(ctx6, req6, nil)
	assert.Error(t, err6)

	var connectErr6 *connect.Error
	assert.ErrorAs(t, err6, &connectErr6)
	assert.Equal(t, connect.CodeResourceExhausted, connectErr6.Code())
	assert.Contains(t, connectErr6.Message(), "max stream connections reached")

	// 2. Test MaxConnsPerSession on a separate user
	user2 := uuid.New()
	sessUser2 := uuid.New()

	// Register 3 streams on the same session (MaxConnsPerSession = 3)
	for i := 0; i < connect_sse.MaxConnsPerSession; i++ {
		connID := uuid.New()
		_, ok := mgr.Register(user2, sessUser2, connID, false)
		assert.True(t, ok, "stream %d registration should succeed", i+1)
	}

	// 4th stream on the same session: should fail with generic max stream connections error
	conn4 := uuid.New()
	ctxStream4 := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UUIDUserID:  user2,
		SessionUUID: sessUser2,
		IsPrimary:   false,
	})

	reqStream4 := connect.NewRequest(&rpc_personal_ssev1.PersonalSseEventsRequest{})
	reqStream4.Header().Set(connect_sse.SSEConnectionIDHeader, conn4.String())

	errStream4 := handler.StreamEvents(ctxStream4, reqStream4, nil)
	assert.Error(t, errStream4)

	var connectErrStream4 *connect.Error
	assert.ErrorAs(t, errStream4, &connectErrStream4)
	assert.Equal(t, connect.CodeResourceExhausted, connectErrStream4.Code())
	assert.Contains(t, connectErrStream4.Message(), "max stream connections reached")
}
