package personal_sse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

const sseNotificationChannel = "personal_sse_events"

// Command types for the Postgres NOTIFY payload matching the 5 cluster manager actions.
const (
	cmdBroadcastToUser           = "broadcast_to_user"            // broadcast to all sessions of a user
	cmdBroadcastToUserExcept     = "broadcast_to_user_except"     // broadcast to all sessions of a user except one
	cmdBroadcastToUserSession    = "broadcast_to_user_session"    // broadcast to one specific session
	cmdUnregisterSession         = "unregister_session"          // close one specific session on all nodes
	cmdUnregisterUserConnections = "unregister_user_connections" // close all sessions of a user on all nodes
)

// postgresSsePayload is the JSON envelope communicated across cluster nodes via pg_notify.
//
// Postgres NOTIFY Limits & Encoding:
//  - PostgreSQL pg_notify strictly accepts a text string with an 8,000-byte limit (PostgreSQL 18 specification).
//  - Binary Protobuf is base64-encoded to prevent C null-byte (\x00) string truncation in Postgres.
//  - 8,000 Byte Limit: Our payloads are ~120 bytes (well under the 8,000-byte limit).
type postgresSsePayload struct {
	Command            string     `json:"cmd"`
	TargetUserID       uuid.UUID  `json:"uid"`
	TargetSessionUUID  *uuid.UUID `json:"tsuid,omitempty"`
	ExcludeSessionUUID *uuid.UUID `json:"esuid,omitempty"`
	ProtoEventBase64   string     `json:"evt,omitempty"`
}

func notifyPostgres(ctx context.Context, pool *pgxpool.Pool, payload postgresSsePayload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal postgres sse payload: %w", err)
	}
	_, err = pool.Exec(ctx, "SELECT pg_notify($1, $2)", sseNotificationChannel, string(payloadBytes))
	return err
}

func marshalEvent(event *rpc_personal_ssev1.PersonalSseEvent) (string, error) {
	eventBytes, err := proto.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal sse event: %w", err)
	}
	return base64.StdEncoding.EncodeToString(eventBytes), nil
}

// publishBroadcastToUser publishes a PersonalSseEvent to all sessions of a user across all cluster nodes via Postgres NOTIFY.
func publishBroadcastToUser(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) error {
	if pool == nil || event == nil {
		return nil
	}
	evtB64, err := marshalEvent(event)
	if err != nil {
		return err
	}
	return notifyPostgres(ctx, pool, postgresSsePayload{
		Command:          cmdBroadcastToUser,
		TargetUserID:     userID,
		ProtoEventBase64: evtB64,
	})
}

// publishBroadcastToUserExcept publishes a PersonalSseEvent to all sessions of a user except excludeSessionUUID across all cluster nodes via Postgres NOTIFY.
func publishBroadcastToUserExcept(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, excludeSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) error {
	if pool == nil || event == nil {
		return nil
	}
	evtB64, err := marshalEvent(event)
	if err != nil {
		return err
	}
	return notifyPostgres(ctx, pool, postgresSsePayload{
		Command:            cmdBroadcastToUserExcept,
		TargetUserID:       userID,
		ExcludeSessionUUID: &excludeSessionUUID,
		ProtoEventBase64:   evtB64,
	})
}

// publishBroadcastToUserSession publishes an event to a single specific session across all cluster nodes via Postgres NOTIFY.
func publishBroadcastToUserSession(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, targetSessionUUID uuid.UUID, event *rpc_personal_ssev1.PersonalSseEvent) error {
	if pool == nil || event == nil {
		return nil
	}
	evtB64, err := marshalEvent(event)
	if err != nil {
		return err
	}
	return notifyPostgres(ctx, pool, postgresSsePayload{
		Command:           cmdBroadcastToUserSession,
		TargetUserID:      userID,
		TargetSessionUUID: &targetSessionUUID,
		ProtoEventBase64:  evtB64,
	})
}

// publishUnregisterSession tells all cluster nodes to close a specific session via Postgres NOTIFY.
func publishUnregisterSession(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, sessionUUID uuid.UUID) error {
	if pool == nil {
		return nil
	}
	return notifyPostgres(ctx, pool, postgresSsePayload{
		Command:           cmdUnregisterSession,
		TargetUserID:      userID,
		TargetSessionUUID: &sessionUUID,
	})
}

// publishUnregisterUserConnections tells all cluster nodes to close all sessions for a user via Postgres NOTIFY.
func publishUnregisterUserConnections(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) error {
	if pool == nil {
		return nil
	}
	return notifyPostgres(ctx, pool, postgresSsePayload{
		Command:      cmdUnregisterUserConnections,
		TargetUserID: userID,
	})
}

// StartPostgresListener runs a resilient, auto-reconnecting loop to listen for NOTIFY events on Postgres channel 'personal_sse_events'.
func StartPostgresListener(ctx context.Context, pool *pgxpool.Pool, sseManager *Manager) {
	if pool == nil || sseManager == nil {
		log.Println("[personal_sse] Skip Postgres Listener: pool or manager is nil")
		return
	}

	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := runListenerSession(ctx, pool, sseManager); err != nil {
			if ctx.Err() != nil {
				return // Normal application shutdown
			}

			log.Printf("[personal_sse] Postgres listener session ended: %v. Reconnecting in %v...", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < 15*time.Second {
					backoff *= 2
				}
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

func runListenerSession(ctx context.Context, pool *pgxpool.Pool, sseManager *Manager) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection for LISTEN: %w", err)
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN "+sseNotificationChannel)
	if err != nil {
		return fmt.Errorf("failed to execute LISTEN %s: %w", sseNotificationChannel, err)
	}

	log.Printf("[personal_sse] Started Postgres Listener on '%s'", sseNotificationChannel)

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("WaitForNotification error: %w", err)
		}

		var payload postgresSsePayload
		if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
			log.Printf("[personal_sse] Ignoring malformed payload: %v", err)
			continue
		}

		if payload.TargetUserID == uuid.Nil {
			log.Printf("[personal_sse] Ignoring payload with nil user_id")
			continue
		}

		switch payload.Command {
		case cmdBroadcastToUser:
			sseEvent, err := unmarshalEvent(payload.ProtoEventBase64)
			if err != nil {
				log.Printf("[personal_sse] Ignoring payload: %v", err)
				continue
			}
			sseManager.Manager.BroadcastToUser(payload.TargetUserID, sseEvent)

		case cmdBroadcastToUserExcept:
			sseEvent, err := unmarshalEvent(payload.ProtoEventBase64)
			if err != nil {
				log.Printf("[personal_sse] Ignoring payload: %v", err)
				continue
			}
			var excludeUUID uuid.UUID
			if payload.ExcludeSessionUUID != nil {
				excludeUUID = *payload.ExcludeSessionUUID
			}
			sseManager.Manager.BroadcastToUserExcept(payload.TargetUserID, excludeUUID, sseEvent)

		case cmdBroadcastToUserSession:
			sseEvent, err := unmarshalEvent(payload.ProtoEventBase64)
			if err != nil {
				log.Printf("[personal_sse] Ignoring payload: %v", err)
				continue
			}
			var targetUUID uuid.UUID
			if payload.TargetSessionUUID != nil {
				targetUUID = *payload.TargetSessionUUID
			}
			sseManager.Manager.BroadcastToUserSession(payload.TargetUserID, targetUUID, sseEvent)

		case cmdUnregisterSession:
			var sessionUUID uuid.UUID
			if payload.TargetSessionUUID != nil {
				sessionUUID = *payload.TargetSessionUUID
			}
			sseManager.Manager.UnregisterSession(payload.TargetUserID, sessionUUID)

		case cmdUnregisterUserConnections:
			sseManager.Manager.UnregisterUserConnections(payload.TargetUserID)

		default:
			log.Printf("[personal_sse] Ignoring unknown command '%s'", payload.Command)
		}
	}
}

func unmarshalEvent(evtB64 string) (*rpc_personal_ssev1.PersonalSseEvent, error) {
	if evtB64 == "" {
		return nil, fmt.Errorf("empty proto_event_base64")
	}
	eventBytes, err := base64.StdEncoding.DecodeString(evtB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 proto: %w", err)
	}
	var sseEvent rpc_personal_ssev1.PersonalSseEvent
	if err := proto.Unmarshal(eventBytes, &sseEvent); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return &sseEvent, nil
}
