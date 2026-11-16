package personal_chat

import (
	rpc_personal_chatv1 "chatbasket-api/gen/proto/personal/personal_chat"
	"chatbasket-api/internal/platform/kit"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcknowledgeDeliveryBatch_ConnectHandlerIntegration verifies the ConnectRPC handler
// delegates cleanly to the batch service and returns the confirmed messages.
func TestAcknowledgeDeliveryBatch_ConnectHandlerIntegration(t *testing.T) {
	mock, svc := newChatMockSvc(t, true)
	server := &chatConnectServer{
		chatService:        svc,
		personalSseManager: nil, // SSE manager nil test
	}

	recipientID := uuid.New()
	msgID1 := uuid.New()
	chatID := uuid.New()
	now := time.Now()

	senderID := uuid.New()
	mock.ExpectQuery(deliveryAckPattern).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows(deliveryAckColumns()).
			AddRow(msgID1, chatID, senderID, recipientID, nil, false, false, false, false, nil, false, now, now.Add(24*time.Hour)))

	batchRows := pgxmock.NewRows(batchAckColumns()).
		AddRow(msgID1, chatID, senderID, nil, true, false, false, false, now)

	mock.ExpectQuery(`(?s)UPDATE messages\s+SET.*delivered_to_recipient_primary = TRUE.*WHERE.*`).
		WithArgs(recipientID, pgxmock.AnyArg()).
		WillReturnRows(batchRows)

	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*WHERE\s+id = \$3`).
		WithArgs(recipientID, now, chatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UserID:      recipientID.String(),
		UUIDUserID:  recipientID,
		SessionID:   "session-token",
		SessionUUID: uuid.New(),
		IsPrimary:   true,
	})

	req := connect.NewRequest(&rpc_personal_chatv1.AckDeliveryBatchPayload{
		MessageIds:     []string{msgID1.String()},
		AcknowledgedBy: "recipient",
	})

	resp, err := server.AcknowledgeDeliveryBatch(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.AcknowledgedCount)
	assert.Equal(t, []string{msgID1.String()}, resp.Msg.AcknowledgedMessageIds)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAcknowledgeDeliveryBatch_IdempotentRetry confirms that when a client retries a batch
// where some messages were already delivered, the endpoint confirms all messages belonging
// to the recipient without emitting duplicate SSE delivery groups.

// TestUnsendMessage_ConnectHandlerIntegration verifies that the ConnectRPC handler returns
// UnsendMessageResponse with AcknowledgedCount and AcknowledgedMessageIds instead of StatusOkay.
func TestUnsendMessage_ConnectHandlerIntegration(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()
	eligibleMsgID := uuid.New()

	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)
	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'`).
		WithArgs(targetChatID, &eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	handler := &chatConnectServer{
		chatService: svc,
	}

	ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UserID:     callerID.StringUserId,
		UUIDUserID: callerID.UuidUserId,
		IsPrimary:  true,
	})
	req := connect.NewRequest(&rpc_personal_chatv1.UnsendMessageRequest{
		MessageIds: []string{eligibleMsgID.String()},
	})

	resp, err := handler.UnsendMessage(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(1), resp.Msg.AcknowledgedCount)
	assert.Equal(t, []string{eligibleMsgID.String()}, resp.Msg.AcknowledgedMessageIds)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_HttpHandlerIntegration verifies that Echo POST /personal/chat/unsend
// returns the batch UnsendMessageResponse with acknowledgedCount and acknowledgedMessageIds.

// TestUnsendMessage_HttpHandlerIntegration verifies that Echo POST /personal/chat/unsend
// returns the batch UnsendMessageResponse with acknowledgedCount and acknowledgedMessageIds.
func TestUnsendMessage_HttpHandlerIntegration(t *testing.T) {
	mock, svc := newChatMockSvc(t, false)

	callerID := kit.UserId{UuidUserId: uuid.New(), StringUserId: uuid.New().String()}
	otherUserID := uuid.New()
	targetChatID := uuid.New()
	eligibleMsgID := uuid.New()
	now := time.Now().Truncate(time.Microsecond)
	futureExpiry := now.Add(24 * time.Hour)

	mock.ExpectQuery(getMessagesForUnsendBatchPattern).
		WithArgs([]uuid.UUID{eligibleMsgID}).
		WillReturnRows(pgxmock.NewRows(getMessagesForUnsendBatchColumns()).
			AddRow(eligibleMsgID, targetChatID, callerID.UuidUserId, otherUserID, "text", nil, false, futureExpiry))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))

	mock.ExpectQuery(`(?s)SELECT.*FROM chats WHERE id = \$1 LIMIT 1 FOR NO KEY UPDATE`).
		WithArgs(targetChatID).
		WillReturnRows(pgxmock.NewRows(chatColumns()).
			AddRow(targetChatID, callerID.UuidUserId, otherUserID, int32(0), int32(0), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, now, now))
	mock.ExpectExec(`(?s)UPDATE messages\s+SET\s+content = 'Message unsent'`).
		WithArgs(eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery(`(?s)INSERT INTO message_sync_actions`).
		WithArgs(pgxmock.AnyArg(), otherUserID, "unsend", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "user_id", "action_type", "payload", "delivered_to_primary", "created_at", "updated_at"}).
			AddRow(uuid.New(), otherUserID, "unsend", []byte("{}"), false, now, now))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_last_message_content = 'Message unsent'`).
		WithArgs(targetChatID, &eligibleMsgID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`(?s)UPDATE chats\s+SET.*p1_unread_count.*WHERE id = \$3`).
		WithArgs(otherUserID, int32(1), targetChatID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	handler := &chatHandler{
		Service: svc,
	}

	e := echo.New()
	body := fmt.Sprintf(`{"messageIds":["%s"]}`, eligibleMsgID)
	req := httptest.NewRequest(http.MethodPost, "/personal/chat/unsend", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("userId", callerID.StringUserId)
	c.Set("uuidUserId", callerID.UuidUserId)
	c.Set("isPrimary", true)

	err := handler.UnsendMessage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var httpResp UnsendMessageResponse
	err = json.Unmarshal(rec.Body.Bytes(), &httpResp)
	require.NoError(t, err)
	assert.Equal(t, 1, httpResp.AcknowledgedCount)
	assert.Equal(t, []string{eligibleMsgID.String()}, httpResp.AcknowledgedMessageIds)

	// Nothing was rejected: the field must still be an empty JSON array, not
	// null, so REST clients can iterate it without a null check.
	var rawResp map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rawResp))
	assert.JSONEq(t, `[]`, string(rawResp["permanentlyRejectedMessageIds"]))

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestUnsendMessage_SyncActions_SecondaryCaller verifies that:
// 1. If caller is secondary (isPrimary = false), a sync action is created for the caller AND for the recipient.
// 2. For the recipient, a sync action is ALWAYS created.

func TestConnectHandlers_RejectOversizedBatches(t *testing.T) {
	uid := uuid.New()
	sid := uuid.New()
	ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
		UserID:      uid.String(),
		UUIDUserID:  uid,
		SessionID:   "session-token",
		SessionUUID: sid,
		IsPrimary:   true,
	})
	s := &chatConnectServer{chatService: &chatService{}}
	ids := oversizedMessageIDStrings(MaxMessageIDsPerRequest + 1)

	cases := []struct {
		name string
		call func() error
	}{
		{"AcknowledgeReadReceiptBatch", func() error {
			_, err := s.AcknowledgeReadReceiptBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckReadReceiptBatchRequest{MessageIds: ids}))
			return err
		}},
		{"AcknowledgeAndReadBatch", func() error {
			_, err := s.AcknowledgeAndReadBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckAndReadBatchPayload{MessageIds: ids}))
			return err
		}},
		{"MarkChatRead", func() error {
			_, err := s.MarkChatRead(ctx, connect.NewRequest(&rpc_personal_chatv1.MarkChatReadRequest{MessageIds: ids}))
			return err
		}},
		{"UnsendMessage", func() error {
			_, err := s.UnsendMessage(ctx, connect.NewRequest(&rpc_personal_chatv1.UnsendMessageRequest{MessageIds: ids}))
			return err
		}},
		{"DeleteMessageForMe", func() error {
			_, err := s.DeleteMessageForMe(ctx, connect.NewRequest(&rpc_personal_chatv1.DeleteMessageForMeRequest{MessageIds: ids}))
			return err
		}},
		{"AcknowledgeDeliveryBatch", func() error {
			_, err := s.AcknowledgeDeliveryBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckDeliveryBatchPayload{MessageIds: ids, AcknowledgedBy: "recipient"}))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

// TestHTTPHandlers_ReturnPermanentlyRejectedMessageIDs verifies that every
// Echo batch handler puts the service's rejected list into its JSON body.
// Malformed IDs never reach the database, so no query is mocked.
func TestHTTPHandlers_ReturnPermanentlyRejectedMessageIDs(t *testing.T) {
	badIDs := []string{"bad-1", "bad-2"}
	idsJSON := `["bad-1","bad-2"]`

	cases := []struct {
		name      string
		body      string
		isPrimary bool
		expect    func(mock pgxmock.PgxPoolIface, caller uuid.UUID)
		call      func(h *chatHandler, c *echo.Context) error
	}{
		{
			name:      "AcknowledgeDeliveryBatch",
			body:      `{"messageIds":` + idsJSON + `,"acknowledgedBy":"recipient"}`,
			isPrimary: true,
			call:      func(h *chatHandler, c *echo.Context) error { return h.AcknowledgeDeliveryBatch(c) },
		},
		{
			name:      "AcknowledgeReadReceiptBatch",
			body:      `{"messageIds":` + idsJSON + `}`,
			isPrimary: true,
			call:      func(h *chatHandler, c *echo.Context) error { return h.AcknowledgeReadReceiptBatch(c) },
		},
		{
			name:      "AcknowledgeAndReadBatch",
			body:      `{"messageIds":` + idsJSON + `}`,
			isPrimary: true,
			call:      func(h *chatHandler, c *echo.Context) error { return h.AcknowledgeAndReadBatch(c) },
		},
		{
			name:      "MarkChatRead",
			body:      `{"messageIds":` + idsJSON + `}`,
			isPrimary: true,
			call:      func(h *chatHandler, c *echo.Context) error { return h.MarkChatRead(c) },
		},
		{
			name:      "UnsendMessage",
			body:      `{"messageIds":` + idsJSON + `}`,
			isPrimary: true,
			call:      func(h *chatHandler, c *echo.Context) error { return h.UnsendMessage(c) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, svc := newChatMockSvc(t, false)
			caller := uuid.New()
			if tc.expect != nil {
				tc.expect(mock, caller)
			}

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("userId", caller.String())
			c.Set("uuidUserId", caller)
			c.Set("isPrimary", tc.isPrimary)
			c.Set("sessionUUID", uuid.New())

			require.NoError(t, tc.call(&chatHandler{Service: svc}, c))
			assert.Equal(t, http.StatusOK, rec.Code)

			var body struct {
				PermanentlyRejectedMessageIds []string `json:"permanentlyRejectedMessageIds"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, badIDs, body.PermanentlyRejectedMessageIds)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestConnectHandlers_ReturnPermanentlyRejectedMessageIDs verifies that every
// ConnectRPC batch handler puts the service's rejected list into its response.
func TestConnectHandlers_ReturnPermanentlyRejectedMessageIDs(t *testing.T) {
	badIDs := []string{"bad-1", "bad-2"}

	cases := []struct {
		name      string
		isPrimary bool
		expect    func(mock pgxmock.PgxPoolIface, caller uuid.UUID)
		call      func(s *chatConnectServer, ctx context.Context) ([]string, error)
	}{
		{
			name:      "AcknowledgeDeliveryBatch",
			isPrimary: true,
			call: func(s *chatConnectServer, ctx context.Context) ([]string, error) {
				resp, err := s.AcknowledgeDeliveryBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckDeliveryBatchPayload{MessageIds: badIDs, AcknowledgedBy: "recipient"}))
				if err != nil {
					return nil, err
				}
				return resp.Msg.PermanentlyRejectedMessageIds, nil
			},
		},
		{
			name:      "AcknowledgeReadReceiptBatch",
			isPrimary: true,
			call: func(s *chatConnectServer, ctx context.Context) ([]string, error) {
				resp, err := s.AcknowledgeReadReceiptBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckReadReceiptBatchRequest{MessageIds: badIDs}))
				if err != nil {
					return nil, err
				}
				return resp.Msg.PermanentlyRejectedMessageIds, nil
			},
		},
		{
			name:      "AcknowledgeAndReadBatch",
			isPrimary: true,
			call: func(s *chatConnectServer, ctx context.Context) ([]string, error) {
				resp, err := s.AcknowledgeAndReadBatch(ctx, connect.NewRequest(&rpc_personal_chatv1.AckAndReadBatchPayload{MessageIds: badIDs}))
				if err != nil {
					return nil, err
				}
				return resp.Msg.PermanentlyRejectedMessageIds, nil
			},
		},
		{
			name:      "MarkChatRead",
			isPrimary: true,
			call: func(s *chatConnectServer, ctx context.Context) ([]string, error) {
				resp, err := s.MarkChatRead(ctx, connect.NewRequest(&rpc_personal_chatv1.MarkChatReadRequest{MessageIds: badIDs}))
				if err != nil {
					return nil, err
				}
				return resp.Msg.PermanentlyRejectedMessageIds, nil
			},
		},
		{
			name:      "UnsendMessage",
			isPrimary: true,
			call: func(s *chatConnectServer, ctx context.Context) ([]string, error) {
				resp, err := s.UnsendMessage(ctx, connect.NewRequest(&rpc_personal_chatv1.UnsendMessageRequest{MessageIds: badIDs}))
				if err != nil {
					return nil, err
				}
				return resp.Msg.PermanentlyRejectedMessageIds, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, svc := newChatMockSvc(t, false)
			caller := uuid.New()
			if tc.expect != nil {
				tc.expect(mock, caller)
			}
			ctx := context.WithValue(context.Background(), kit.CtxSessionData, kit.SessionData{
				UserID:      caller.String(),
				UUIDUserID:  caller,
				SessionID:   "session-token",
				SessionUUID: uuid.New(),
				IsPrimary:   tc.isPrimary,
			})

			got, err := tc.call(&chatConnectServer{chatService: svc}, ctx)
			require.NoError(t, err)
			assert.Equal(t, badIDs, got)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
