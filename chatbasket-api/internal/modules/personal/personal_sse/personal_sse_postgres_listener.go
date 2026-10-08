package personal_sse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	rpc_personal_ssev1 "chatbasket-api/gen/proto/personal/personal_sse"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const sseNotificationChannel = "personal_sse_events"

// Command types for the Postgres NOTIFY payload matching the 5 cluster manager actions.
const (
	cmdBroadcastToUser           = "broadcast_to_user"           // broadcast to all sessions of a user
	cmdBroadcastToUserExcept     = "broadcast_to_user_except"    // broadcast to all sessions of a user except one
	cmdBroadcastToUserSession    = "broadcast_to_user_session"   // broadcast to one specific session
	cmdUnregisterSession         = "unregister_session"          // close one specific session on all nodes
	cmdUnregisterUserConnections = "unregister_user_connections" // close all sessions of a user on all nodes
)

// postgresSsePayload is the JSON message sent between servers using pg_notify.
//
// Size rules:
//   - Postgres drops any NOTIFY message bigger than 8,000 bytes.
//   - The event is first packed with protobuf, then changed to base64 text
//     (about 33% bigger), then put inside JSON. So the final message is
//     bigger than the event content alone.
//   - Small events (delivery ack, read, delete) are about 225 bytes and fit.
//   - All events share one general 64KB ceiling (see maxEventBytes).
//     Payloads over the inline threshold route via the personal_sse_outbox
//     table with only the row id in NOTIFY (see routeAndNotify).
type postgresSsePayload struct {
	Command            string     `json:"cmd"`
	TargetUserID       uuid.UUID  `json:"uid"`
	TargetSessionUUID  *uuid.UUID `json:"tsuid,omitempty"`
	ExcludeSessionUUID *uuid.UUID `json:"esuid,omitempty"`
	ProtoEventBase64   string     `json:"evt,omitempty"`
	OutboxID           *uuid.UUID `json:"oid,omitempty"`
}

// StartListener runs a resilient, auto-reconnecting loop to listen for NOTIFY events on Postgres channel 'personal_sse_events'.
// A pool of outbox workers drains large-event hydration concurrently so
// slow DB fetches never block urgent small events in the receive loop.
func (m *Manager) StartListener(ctx context.Context) {
	if m == nil || m.pool == nil {
		log.Println("[personal_sse] Skip Postgres Listener: pool or manager is nil")
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < outboxWorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.outboxWorker(ctx)
		}()
	}
	defer wg.Wait()

	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := m.runListenerSession(ctx); err != nil {
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

func (m *Manager) runListenerSession(ctx context.Context) error {
	conn, err := m.pool.Acquire(ctx)
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

		// Large events go to background workers (never blocks the loop);
		// inline payloads decode from base64 here. Unregister commands carry
		// no event and skip decoding. Missing/expired rows are skipped;
		// the client falls back to GetMessages/GetPendingMessages polling.
		var sseEvent *rpc_personal_ssev1.PersonalSseEvent
		switch payload.Command {
		case cmdBroadcastToUser, cmdBroadcastToUserExcept, cmdBroadcastToUserSession:
			if payload.OutboxID != nil {
				// Large event: hand to background workers without blocking.
				// Filter first so other nodes' mail never touches our DB.
				if !m.hasLocalTarget(payload) {
					continue
				}
				if !m.enqueueOutbox(payload) {
					log.Printf("[personal_sse] Outbox queue full, dropping large event")
				}
				continue
			}
			var derr error
			sseEvent, derr = unmarshalEvent(payload.ProtoEventBase64)
			if derr != nil {
				log.Printf("[personal_sse] Ignoring payload: %v", derr)
				continue
			}
		}

		m.dispatchResolved(payload, sseEvent)
	}
}

// enqueueOutbox offers an outbox pointer to background workers without
// blocking the listener loop. False means the queue is full: the event
// is dropped here and the client falls back to polling.
func (m *Manager) enqueueOutbox(payload postgresSsePayload) bool {
	select {
	case m.outboxQueue <- payload:
		return true
	default:
		return false
	}
}

// outboxWorker hydrates large events concurrently so slow fetches never
// block the listener loop. Exits on context cancel; queued items are
// dropped on shutdown (polling covers). Never closes the queue channel:
// the loop may still send during shutdown races, and send-on-closed panics.
func (m *Manager) outboxWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-m.outboxQueue:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[personal_sse] Outbox worker recovered: %v", r)
					}
				}()
				m.processOutboxPayload(ctx, payload)
			}()
		}
	}
}

// processOutboxPayload fetches one queued large event and delivers it
// locally. Re-checks presence first: the check in the loop may be stale
// by the time a worker gets here.
func (m *Manager) processOutboxPayload(ctx context.Context, payload postgresSsePayload) {
	if payload.OutboxID == nil || !m.hasLocalTarget(payload) {
		return
	}
	// Deadline nests under the worker context: server shutdown cancels
	// an inflight fetch at once instead of waiting it out.
	fctx, cancel := context.WithTimeout(ctx, publishTimeout)
	eventBytes, err := m.postgresQueries.GetSseOutboxRow(fctx, *payload.OutboxID)
	cancel()
	if err != nil {
		log.Printf("[personal_sse] Ignoring missing outbox row %s: %v", payload.OutboxID, err)
		return
	}
	sseEvent, err := unmarshalEventBytes(eventBytes)
	if err != nil {
		log.Printf("[personal_sse] Ignoring payload: %v", err)
		return
	}
	m.dispatchResolved(payload, sseEvent)
}

// dispatchResolved hands a ready event to the matching streams on this
// server, the last step before it reaches the connected user client.
func (m *Manager) dispatchResolved(payload postgresSsePayload, sseEvent *rpc_personal_ssev1.PersonalSseEvent) {
	switch payload.Command {
	case cmdBroadcastToUser:
		m.Manager.BroadcastToUser(payload.TargetUserID, sseEvent)

	case cmdBroadcastToUserExcept:
		var excludeUUID uuid.UUID
		if payload.ExcludeSessionUUID != nil {
			excludeUUID = *payload.ExcludeSessionUUID
		}
		m.Manager.BroadcastToUserExcept(payload.TargetUserID, excludeUUID, sseEvent)

	case cmdBroadcastToUserSession:
		var targetUUID uuid.UUID
		if payload.TargetSessionUUID != nil {
			targetUUID = *payload.TargetSessionUUID
		}
		m.Manager.BroadcastToUserSession(payload.TargetUserID, targetUUID, sseEvent)

	case cmdUnregisterSession:
		var sessionUUID uuid.UUID
		if payload.TargetSessionUUID != nil {
			sessionUUID = *payload.TargetSessionUUID
		}
		m.Manager.UnregisterSession(payload.TargetUserID, sessionUUID)

	case cmdUnregisterUserConnections:
		m.Manager.UnregisterUserConnections(payload.TargetUserID)

	default:
		log.Printf("[personal_sse] Ignoring unknown command '%s'", payload.Command)
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
	return unmarshalEventBytes(eventBytes)
}

// unmarshalEventBytes decodes raw proto bytes from the outbox table.
// No base64 step: BYTEA carries the binary event natively.
func unmarshalEventBytes(eventBytes []byte) (*rpc_personal_ssev1.PersonalSseEvent, error) {
	var sseEvent rpc_personal_ssev1.PersonalSseEvent
	if err := proto.Unmarshal(eventBytes, &sseEvent); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return &sseEvent, nil
}

// hasLocalTarget checks if this local node hosts any active connection matching the payload's recipient target.
func (m *Manager) hasLocalTarget(payload postgresSsePayload) bool {
	switch payload.Command {
	case cmdBroadcastToUser:
		return m.Manager.HasUser(payload.TargetUserID)
	case cmdBroadcastToUserExcept:
		if payload.ExcludeSessionUUID == nil {
			return m.Manager.HasUser(payload.TargetUserID)
		}
		return m.Manager.HasUserExcept(payload.TargetUserID, *payload.ExcludeSessionUUID)
	case cmdBroadcastToUserSession:
		if payload.TargetSessionUUID == nil {
			return false
		}
		return m.Manager.IsSessionActive(payload.TargetUserID, *payload.TargetSessionUUID)
	default:
		return true
	}
}
