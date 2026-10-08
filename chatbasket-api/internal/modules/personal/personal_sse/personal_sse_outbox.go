package personal_sse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"time"
)

// Smart routing: Postgres drops NOTIFY over 8000 bytes.
// Route only large payloads via the outbox table, keep small
// events inline. 7000 leaves margin for channel name + JSON growth.
const outboxInlineLimit = 7000

// maxEventBytes is the hard ceiling for one SSE event. Anything bigger is
// rejected before any database write or NOTIFY. 64KB clears the largest
// legitimate message (~16KB by E2EE math) with wide margin, while blocking
// runaway or malicious payloads from bloating the outbox table, worker
// memory, and every node's fetch.
const maxEventBytes = 64 * 1024

// Outbox row lifetime: only needs to cover NOTIFY lag + listener
// reconnect (backoff caps at 15s). 3 minutes keeps the table tiny
// and stays well under the 15-minute R2 presigned URL lifetime stored
// inside file message events.
const outboxTTL = 3 * time.Minute
const outboxSweepInterval = 5 * time.Minute

// publishTimeout bounds user-facing fanout DB calls (INSERT/NOTIFY/fetch).
// A stalled database aborts instead of parking a goroutine forever.
// Missed events are harmless: the client falls back to polling.
// 10 seconds tolerates cold database wakes; beyond that it is a real
// outage and polling is the right path anyway.
const publishTimeout = 10 * time.Second

// sweepTimeout bounds the background orphan cleanup. Generous on purpose:
// nobody waits on the sweeper, and an outage can pile up rows to clear.
const sweepTimeout = 4 * time.Minute

// inlineSizeEstimate returns the final NOTIFY JSON size for raw proto
// bytes without building the big string: official base64 length plus
// the exact envelope for this payload shape, plus the 9 framing bytes
// around a non-empty evt value ("evt":"...",). Empty events omit evt.
func inlineSizeEstimate(payload postgresSsePayload, protoLen int) int {
	stripped := payload
	stripped.ProtoEventBase64 = ""
	env, err := json.Marshal(stripped)
	if err != nil {
		return outboxInlineLimit + 1
	}
	est := len(env)
	if protoLen > 0 {
		est += base64.StdEncoding.EncodedLen(protoLen) + len(`"evt":"",`)
	}
	return est
}

// shouldUseOutbox reports whether raw proto bytes exceed the inline
// limit and must go via the outbox table.
func shouldUseOutbox(payload postgresSsePayload, eventBytes []byte) bool {
	return inlineSizeEstimate(payload, len(eventBytes)) > outboxInlineLimit
}

// sweepBatchSize bounds one DELETE so a huge orphan pile makes progress
// batch by batch instead of rolling back one giant statement on timeout.
const sweepBatchSize = 1000

// StartSweeper deletes expired outbox rows periodically. Rows are retained
// for outboxTTL to allow all cluster nodes to hydrate during multi-node fanout.
func (m *Manager) StartSweeper(ctx context.Context) {
	if m == nil || m.pool == nil {
		return
	}
	ticker := time.NewTicker(outboxSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sctx, scancel := context.WithTimeout(ctx, sweepTimeout)
			for {
				n, err := m.postgresQueries.DeleteExpiredSseOutboxBatch(sctx, sweepBatchSize)
				if err != nil {
					log.Printf("[personal_sse] outbox sweep failed: %v", err)
					break
				}
				if n == 0 || sctx.Err() != nil {
					break
				}
			}
			scancel()
		}
	}
}
