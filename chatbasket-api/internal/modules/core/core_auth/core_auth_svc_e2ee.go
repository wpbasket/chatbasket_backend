package core_auth

import (
	"fmt"
	"chatbasket-api/internal/modules/core/core_auth/internal/core_auth_store"
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)



func (s *AuthService) SaveSessionE2EEPublicKey(ctx context.Context, tx pgx.Tx, userID uuid.UUID, sessionID uuid.UUID, publicKey string) error {
	queries := s.PostgresQueries
	if tx != nil {
		queries = s.PostgresQueries.WithTx(tx)
	}
	_, err := queries.SaveSessionE2EEPublicKey(ctx, core_auth_store.SaveSessionE2EEPublicKeyParams{
		E2eePublicKey: &publicKey,
		ID:            sessionID,
		AuthUserID:    userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Printf("[E2EE] SaveSessionE2EEPublicKey: session NOT FOUND for user %s session %s", userID, sessionID)
			return fmt.Errorf("session not found or expired")
		}
		log.Printf("[E2EE] SaveSessionE2EEPublicKey: DB error for user %s: %v", userID, err)
		return err
	}
	log.Printf("[E2EE] SaveSessionE2EEPublicKey: saved key for user %s session %s", userID, sessionID)
	return nil
}

func (s *AuthService) GetActiveSessionKeysForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.PostgresQueries.GetActiveSessionKeysForUser(ctx, userID)
	if err != nil {
		log.Printf("[E2EE] GetActiveSessionKeysForUser: DB error for user %s: %v", userID, err)
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.E2eePublicKey != nil {
			keys = append(keys, *row.E2eePublicKey)
		}
	}
	log.Printf("[E2EE] GetActiveSessionKeysForUser: user %s → %d active keyed session(s)", userID, len(keys))
	return keys, nil
}

// GetActiveSessionKeysForUserExcluding returns all active session keys for a user,
// excluding the caller's own session (so a user querying their own keys doesn't
// get their own key back — it's already in their SecureStore).
func (s *AuthService) GetActiveSessionKeysForUserExcluding(ctx context.Context, userID uuid.UUID, excludeSessionID uuid.UUID) ([]string, error) {
	rows, err := s.PostgresQueries.GetActiveSessionKeysForUserExcluding(ctx, core_auth_store.GetActiveSessionKeysForUserExcludingParams{
		AuthUserID: userID,
		ID:         excludeSessionID,
	})
	if err != nil {
		log.Printf("[E2EE] GetActiveSessionKeysForUserExcluding: DB error for user %s: %v", userID, err)
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.E2eePublicKey != nil {
			keys = append(keys, *row.E2eePublicKey)
		}
	}
	log.Printf("[E2EE] GetActiveSessionKeysForUserExcluding: user %s (excl session %s) → %d sibling key(s)", userID, excludeSessionID, len(keys))
	return keys, nil
}

func (s *AuthService) CountActiveKeyedSessionsForUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (int64, error) {
	queries := s.PostgresQueries
	if tx != nil {
		queries = s.PostgresQueries.WithTx(tx)
	}
	return queries.CountActiveKeyedSessionsForUser(ctx, userID)
}

func (s *AuthService) GetKeysRevision(ctx context.Context, userID uuid.UUID) (int32, error) {
	rev, err := s.PostgresQueries.GetKeysRevision(ctx, userID)
	if err != nil {
		log.Printf("[E2EE] GetKeysRevision: DB error for user %s: %v", userID, err)
		return 0, err
	}
	return rev, nil
}

func (s *AuthService) GetKeysRevisions(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]int32, error) {
	if len(userIDs) == 0 {
		return map[uuid.UUID]int32{}, nil
	}
	rows, err := s.PostgresQueries.GetKeysRevisions(ctx, userIDs)
	if err != nil {
		log.Printf("[E2EE] GetKeysRevisions: DB error: %v", err)
		return nil, err
	}
	res := make(map[uuid.UUID]int32, len(rows))
	for _, r := range rows {
		res[r.ID] = r.KeysRevision
	}
	return res, nil
}


func (s *AuthService) IncrementKeysRevision(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	queries := s.PostgresQueries
	if tx != nil {
		queries = s.PostgresQueries.WithTx(tx)
	}
	if err := queries.IncrementKeysRevision(ctx, userID); err != nil {
		log.Printf("[E2EE] IncrementKeysRevision: FAILED for user %s: %v", userID, err)
		return err
	}
	log.Printf("[E2EE] IncrementKeysRevision: user %s revision incremented", userID)
	return nil
}

func (s *AuthService) ResetKeysRevision(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	queries := s.PostgresQueries
	if tx != nil {
		queries = s.PostgresQueries.WithTx(tx)
	}
	if err := queries.ResetKeysRevision(ctx, userID); err != nil {
		log.Printf("[E2EE] ResetKeysRevision: FAILED for user %s: %v", userID, err)
		return err
	}
	log.Printf("[E2EE] ResetKeysRevision: user %s revision reset to 0", userID)
	return nil
}
