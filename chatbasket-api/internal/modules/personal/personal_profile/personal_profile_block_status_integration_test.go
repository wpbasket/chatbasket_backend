package personal_profile

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"chatbasket-api/internal/modules/personal/personal_profile/internal/personal_profile_store"
	"chatbasket-api/internal/platform/kit"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlockedBetweenUsers_Integration_NoBlock(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.False(t, status.IsBlocked)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsers_Integration_RequesterAdminBlocked(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", requester.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.True(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsers_Integration_TargetAdminBlocked(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", target.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.True(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsers_Integration_RequesterBlockedByTarget(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), target.UuidUserId, requester.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.True(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsers_Integration_TargetBlockedByRequester(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.True(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsers_Integration_AllConditionsTrue(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", requester.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", target.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), target.UuidUserId, requester.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.True(t, status.IsRequesterAdminBlocked)
	assert.True(t, status.IsTargetAdminBlocked)
	assert.True(t, status.IsRequesterUserBlockedByTarget)
	assert.True(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsersBatch_Integration_EmptyList(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{})
	require.NoError(t, err)
	assert.Empty(t, statuses)
}

func TestIsBlockedBetweenUsersBatch_Integration_AllUnblocked(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target1, _ := createTestUserWithProfile(t, pool)
	target2, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target1.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target2.UuidUserId) })

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{target1.UuidUserId, target2.UuidUserId})
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for _, s := range statuses {
		assert.False(t, s.IsBlocked)
		assert.False(t, s.IsRequesterAdminBlocked)
		assert.False(t, s.IsTargetAdminBlocked)
		assert.False(t, s.IsRequesterUserBlockedByTarget)
		assert.False(t, s.IsTargetUserBlockedByRequester)
	}
}

func TestIsBlockedBetweenUsersBatch_Integration_MixedStatuses(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	targetUnblocked, _ := createTestUserWithProfile(t, pool)
	targetAdminBlocked, _ := createTestUserWithProfile(t, pool)
	targetBlockedByRequester, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", targetUnblocked.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", targetAdminBlocked.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", targetBlockedByRequester.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", targetAdminBlocked.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, targetBlockedByRequester.UuidUserId)
	require.NoError(t, err)

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{
		targetUnblocked.UuidUserId,
		targetAdminBlocked.UuidUserId,
		targetBlockedByRequester.UuidUserId,
	})
	require.NoError(t, err)
	require.Len(t, statuses, 3)

	byID := make(map[uuid.UUID]*BlockStatusResult)
	for _, s := range statuses {
		byID[s.TargetID] = s
	}
	assert.False(t, byID[targetUnblocked.UuidUserId].IsTargetAdminBlocked)
	assert.False(t, byID[targetUnblocked.UuidUserId].IsTargetUserBlockedByRequester)
	assert.True(t, byID[targetAdminBlocked.UuidUserId].IsTargetAdminBlocked)
	assert.False(t, byID[targetAdminBlocked.UuidUserId].IsTargetUserBlockedByRequester)
	assert.False(t, byID[targetBlockedByRequester.UuidUserId].IsTargetAdminBlocked)
	assert.True(t, byID[targetBlockedByRequester.UuidUserId].IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsersBatch_Integration_RequesterAdminBlocked(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target1, _ := createTestUserWithProfile(t, pool)
	target2, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target1.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target2.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", requester.UuidUserId)
	require.NoError(t, err)

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{target1.UuidUserId, target2.UuidUserId})
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for _, s := range statuses {
		assert.True(t, s.IsBlocked)
		assert.True(t, s.IsRequesterAdminBlocked)
	}
}

func TestIsBlockedBetweenUsersBatch_Integration_Duplicates(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", target.UuidUserId)
	require.NoError(t, err)

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{target.UuidUserId, target.UuidUserId})
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for _, s := range statuses {
		assert.True(t, s.IsBlocked)
		assert.True(t, s.IsTargetAdminBlocked)
	}
}

func TestQueriesIsBlockedBetweenUsers_Integration_Direct(t *testing.T) {
	pool, _, _ := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	queries := personal_profile_store.New(pool)

	// No block
	row, err := queries.IsBlockedBetweenUsers(ctx, personal_profile_store.IsBlockedBetweenUsersParams{
		RequesterUserID: requester.UuidUserId,
		TargetUserID:    target.UuidUserId,
	})
	require.NoError(t, err)
	assert.False(t, row.RequesterAdminBlocked)
	assert.False(t, row.TargetAdminBlocked)
	assert.False(t, row.RequesterUserBlockedByTarget)
	assert.False(t, row.TargetUserBlockedByRequester)

	// Target blocked by requester
	_, err = pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)

	row, err = queries.IsBlockedBetweenUsers(ctx, personal_profile_store.IsBlockedBetweenUsersParams{
		RequesterUserID: requester.UuidUserId,
		TargetUserID:    target.UuidUserId,
	})
	require.NoError(t, err)
	assert.True(t, row.TargetUserBlockedByRequester)
	assert.False(t, row.RequesterAdminBlocked)
	assert.False(t, row.TargetAdminBlocked)
	assert.False(t, row.RequesterUserBlockedByTarget)
}

func TestQueriesIsBlockedBetweenUsersBatch_Integration_Direct(t *testing.T) {
	pool, _, _ := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target1, _ := createTestUserWithProfile(t, pool)
	target2, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target1.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target2.UuidUserId) })

	_, err := pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", target1.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, target2.UuidUserId)
	require.NoError(t, err)

	queries := personal_profile_store.New(pool)
	rows, err := queries.IsBlockedBetweenUsersBatch(ctx, personal_profile_store.IsBlockedBetweenUsersBatchParams{
		RequesterUserID: requester.UuidUserId,
		TargetUserIds:   []uuid.UUID{target1.UuidUserId, target2.UuidUserId},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byID := make(map[uuid.UUID]personal_profile_store.IsBlockedBetweenUsersBatchRow)
	for _, r := range rows {
		byID[r.TargetID] = r
	}
	assert.True(t, byID[target1.UuidUserId].TargetAdminBlocked)
	assert.False(t, byID[target1.UuidUserId].TargetUserBlockedByRequester)
	assert.True(t, byID[target2.UuidUserId].TargetUserBlockedByRequester)
	assert.False(t, byID[target2.UuidUserId].TargetAdminBlocked)
}

func TestNewErrorWithDetails_BlockStatus_Integration_RoundTrip(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
		uuid.New(), requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)

	err = kit.NewErrorWithDetails(http.StatusForbidden, "forbidden", "blocked", BlockStatusFlags{
		IsRequesterAdminBlocked:        status.IsRequesterAdminBlocked,
		IsTargetAdminBlocked:           status.IsTargetAdminBlocked,
		IsRequesterUserBlockedByTarget: status.IsRequesterUserBlockedByTarget,
		IsTargetUserBlockedByRequester: status.IsTargetUserBlockedByRequester,
	})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusForbidden, processed.Status())
	assert.Equal(t, "blocked", processed.Error())
}
