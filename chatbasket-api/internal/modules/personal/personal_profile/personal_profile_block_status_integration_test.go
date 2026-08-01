package personal_profile

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
		IsTargetProfilePrivate:         status.IsTargetProfilePrivate,
	})
	require.Error(t, err)
	var processed kit.ProcessedError
	require.True(t, errors.As(err, &processed))
	assert.Equal(t, http.StatusForbidden, processed.Status())
	assert.Equal(t, "blocked", processed.Error())
}

func TestIsBlockedBetweenUsers_Integration_TargetPrivate(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	// Update target's profile type to 'private'
	_, err := pool.Exec(ctx, "UPDATE users SET profile_type = 'private' WHERE id = $1", target.UuidUserId)
	require.NoError(t, err)

	status, err := profileSvc.IsBlockedBetweenUsers(ctx, requester.UuidUserId, target.UuidUserId)
	require.NoError(t, err)
	assert.True(t, status.IsBlocked)
	assert.True(t, status.IsTargetProfilePrivate)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestIsBlockedBetweenUsersBatch_Integration_TargetPrivate(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	target, _ := createTestUserWithProfile(t, pool)
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", requester.UuidUserId) })
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", target.UuidUserId) })

	// Update target's profile type to 'private'
	_, err := pool.Exec(ctx, "UPDATE users SET profile_type = 'private' WHERE id = $1", target.UuidUserId)
	require.NoError(t, err)

	statuses, err := profileSvc.IsBlockedBetweenUsersBatch(ctx, requester.UuidUserId, []uuid.UUID{target.UuidUserId})
	require.NoError(t, err)
	require.Len(t, statuses, 1)

	status := statuses[0]
	assert.True(t, status.IsBlocked)
	assert.True(t, status.IsTargetProfilePrivate)
	assert.False(t, status.IsRequesterAdminBlocked)
	assert.False(t, status.IsTargetAdminBlocked)
	assert.False(t, status.IsRequesterUserBlockedByTarget)
	assert.False(t, status.IsTargetUserBlockedByRequester)
}

func TestGetContactableProfilesForViewer_Integration_ExcludesBothBlockDirections(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	ownBlocked, _ := createTestUserWithProfile(t, pool)
	reciprocalBlocked, _ := createTestUserWithProfile(t, pool)
	visible, _ := createTestUserWithProfile(t, pool)
	for _, user := range []kit.UserId{requester, ownBlocked, reciprocalBlocked, visible} {
		userID := user.UuidUserId
		t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", userID) })
	}

	_, err := pool.Exec(ctx,
		"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3), ($4, $5, $6)",
		uuid.New(), requester.UuidUserId, ownBlocked.UuidUserId,
		uuid.New(), reciprocalBlocked.UuidUserId, requester.UuidUserId,
	)
	require.NoError(t, err)

	profiles, err := profileSvc.GetContactableProfilesForViewer(ctx, requester.UuidUserId, []uuid.UUID{
		ownBlocked.UuidUserId,
		reciprocalBlocked.UuidUserId,
		visible.UuidUserId,
	})
	require.NoError(t, err)

	assert.NotContains(t, profiles, ownBlocked.UuidUserId)
	assert.NotContains(t, profiles, reciprocalBlocked.UuidUserId)
	assert.Contains(t, profiles, visible.UuidUserId)

	contactableIDs, err := profileSvc.GetContactableUserIDs(ctx, requester.UuidUserId, []uuid.UUID{
		ownBlocked.UuidUserId,
		reciprocalBlocked.UuidUserId,
		visible.UuidUserId,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{visible.UuidUserId}, contactableIDs)
}

func TestGetBlockListProfilesForViewer_Integration_PrivacyAndOrdering(t *testing.T) {
	pool, _, profileSvc := setupProfileIntegrationDB(t)
	ctx := context.Background()

	requester, _ := createTestUserWithProfile(t, pool)
	ownBlocked, _ := createTestUserWithProfile(t, pool)
	reciprocalBlocked, _ := createTestUserWithProfile(t, pool)
	privateTarget, _ := createTestUserWithProfile(t, pool)
	adminBlocked, _ := createTestUserWithProfile(t, pool)
	for _, user := range []kit.UserId{requester, ownBlocked, reciprocalBlocked, privateTarget, adminBlocked} {
		userID := user.UuidUserId
		t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM auth_users WHERE id = $1", userID) })
	}

	ownBio := "own block bio"
	reciprocalBio := "reciprocal block bio"
	_, err := pool.Exec(ctx, "UPDATE users SET name = $1, bio = $2 WHERE id = $3", "Own Blocked", ownBio, ownBlocked.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "UPDATE users SET name = $1, bio = $2 WHERE id = $3", "Reciprocal Blocked", reciprocalBio, reciprocalBlocked.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "UPDATE users SET profile_type = 'private' WHERE id = $1", privateTarget.UuidUserId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "UPDATE users SET is_admin_blocked = true WHERE id = $1", adminBlocked.UuidUserId)
	require.NoError(t, err)

	insertBlock := func(blockerID, blockedID uuid.UUID) {
		t.Helper()
		_, err := pool.Exec(ctx,
			"INSERT INTO user_blocks (id, blocker_user_id, blocked_user_id) VALUES ($1, $2, $3)",
			uuid.New(), blockerID, blockedID)
		require.NoError(t, err)
	}
	// user_blocks' timestamp trigger owns created_at, so insert rows separately
	// and let the short delays establish a deterministic newest-first order.
	insertBlock(requester.UuidUserId, adminBlocked.UuidUserId)
	time.Sleep(20 * time.Millisecond)
	insertBlock(requester.UuidUserId, privateTarget.UuidUserId)
	time.Sleep(20 * time.Millisecond)
	insertBlock(requester.UuidUserId, reciprocalBlocked.UuidUserId)
	time.Sleep(20 * time.Millisecond)
	insertBlock(requester.UuidUserId, ownBlocked.UuidUserId)
	insertBlock(reciprocalBlocked.UuidUserId, requester.UuidUserId)

	blocks, err := profileSvc.GetUserBlocks(ctx, requester.UuidUserId)
	require.NoError(t, err)
	require.Len(t, blocks, 4)
	assert.Equal(t, ownBlocked.UuidUserId, blocks[0].BlockedUserID)
	assert.Equal(t, reciprocalBlocked.UuidUserId, blocks[1].BlockedUserID)
	assert.Equal(t, privateTarget.UuidUserId, blocks[2].BlockedUserID)
	assert.Equal(t, adminBlocked.UuidUserId, blocks[3].BlockedUserID)
	assert.True(t, blocks[0].CreatedAt.After(blocks[1].CreatedAt))
	assert.True(t, blocks[1].CreatedAt.After(blocks[2].CreatedAt))
	assert.True(t, blocks[2].CreatedAt.After(blocks[3].CreatedAt))

	profiles, err := profileSvc.GetBlockListProfilesForViewer(ctx, requester.UuidUserId, []uuid.UUID{
		ownBlocked.UuidUserId,
		reciprocalBlocked.UuidUserId,
		privateTarget.UuidUserId,
		adminBlocked.UuidUserId,
	})
	require.NoError(t, err)

	ownProfile, ok := profiles[ownBlocked.UuidUserId]
	require.True(t, ok)
	assert.Equal(t, "Own Blocked", ownProfile.Name)
	assert.Equal(t, "user", ownProfile.Username)
	assert.Equal(t, "public", ownProfile.ProfileType)
	require.NotNil(t, ownProfile.Bio)
	assert.Equal(t, ownBio, *ownProfile.Bio)

	reciprocalProfile, ok := profiles[reciprocalBlocked.UuidUserId]
	require.True(t, ok)
	assert.Equal(t, "Reciprocal Blocked", reciprocalProfile.Name)
	assert.Equal(t, "user", reciprocalProfile.Username)
	assert.Equal(t, "public", reciprocalProfile.ProfileType)
	assert.Nil(t, reciprocalProfile.Bio)
	assert.Nil(t, reciprocalProfile.AvatarURL)
	assert.Nil(t, reciprocalProfile.AvatarFileId)

	assert.NotContains(t, profiles, privateTarget.UuidUserId)
	assert.NotContains(t, profiles, adminBlocked.UuidUserId)
}
