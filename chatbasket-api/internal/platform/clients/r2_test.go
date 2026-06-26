package clients

import (
	appconfig "chatbasket-api/internal/platform/config"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestR2ClientPool_Routing(t *testing.T) {
	configs := []appconfig.R2AccountConfig{
		{
			Name:             "alpha",
			AccountID:        "11111111111111111111111111111111",
			AccessKeyID:      "mock-access-key-1",
			SecretAccessKey:  "mock-secret-key-1",
			ChatFilesBucket:  "alpha-chat-bucket",
			ProfilePicBucket: "alpha-profile-bucket",
		},
		{
			Name:             "beta",
			AccountID:        "22222222222222222222222222222222",
			AccessKeyID:      "mock-access-key-2",
			SecretAccessKey:  "mock-secret-key-2",
			ChatFilesBucket:  "beta-chat-bucket",
			ProfilePicBucket: "beta-profile-bucket",
		},
	}

	pool, err := NewR2ClientPool(&appconfig.R2PoolConfig{
		Accounts:              configs,
		PrimaryChatAccount:    "alpha",
		PrimaryProfileAccount: "alpha",
	})
	require.NoError(t, err)
	require.NotNil(t, pool)

	// 1. Test GetClientByAccount matches exactly
	alphaClient := pool.GetClientByAccount("alpha")
	require.NotNil(t, alphaClient)
	assert.Equal(t, "alpha-chat-bucket", alphaClient.ChatBucket())

	betaClient := pool.GetClientByAccount("beta")
	require.NotNil(t, betaClient)
	assert.Equal(t, "beta-chat-bucket", betaClient.ChatBucket())

	// 2. Test GetClientByAccount fallback for retired/unknown account name
	fallbackClient := pool.GetClientByAccount("retired-gamma")
	require.NotNil(t, fallbackClient)
	assert.Equal(t, "alpha-chat-bucket", fallbackClient.ChatBucket()) // Falls back to primary

	// 3. Test GetClientByAccount fallback for empty string
	emptyClient := pool.GetClientByAccount("")
	require.NotNil(t, emptyClient)
	assert.Equal(t, "alpha-chat-bucket", emptyClient.ChatBucket()) // Falls back to primary

	// 4. Test GetClient with prefix parses and routes correctly
	routedClient := pool.GetClient("beta:some-uuid")
	require.NotNil(t, routedClient)
	assert.Equal(t, "beta-chat-bucket", routedClient.ChatBucket())

	// 5. Test GetClient fallback for prefix-less file ID
	prefixlessClient := pool.GetClient("legacy-uuid-without-prefix")
	require.NotNil(t, prefixlessClient)
	assert.Equal(t, "alpha-chat-bucket", prefixlessClient.ChatBucket())
}
