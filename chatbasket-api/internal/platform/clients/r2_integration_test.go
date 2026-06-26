package clients

import (
	"bytes"
	appconfig "chatbasket-api/internal/platform/config"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestR2Client_RealIntegration(t *testing.T) {
	accountID := os.Getenv("R2_ACCOUNT_ID")
	if accountID == "" {
		accountID = "dummy-account-id"
	}
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	if accessKey == "" {
		accessKey = "dummy-access-key"
	}
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	if secretKey == "" {
		secretKey = "dummy-secret-key"
	}

	if accountID == "dummy-account-id" || accessKey == "dummy-access-key" {
		t.Skip("Skipping R2 integration test: dummy credentials used")
	}

	ctx := context.Background()

	// 1. Initialize R2 Account Config
	cfg := &appconfig.R2AccountConfig{
		Name:             "test-integration",
		AccountID:        accountID,
		AccessKeyID:      accessKey,
		SecretAccessKey:  secretKey,
		ChatFilesBucket:  "integration-chat-bucket",
		ProfilePicBucket: "integration-profile-bucket",
	}

	client, err := NewR2Client(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// 2. Use the user's bucket name directly
	targetBucket := "chat-files"
	client.ChatFilesBucket = targetBucket

	// 3. Test File Upload Flow
	testKey := fmt.Sprintf("test-integration-%s.txt", uuid.New().String())
	content := []byte("Cloudflare R2 Integration Test Content - AntiGravity")
	body := bytes.NewReader(content)

	t.Logf("Uploading test file to R2: %s", testKey)
	err = client.UploadFile(ctx, targetBucket, testKey, body, "text/plain")
	if err != nil {
		if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "403") {
			t.Skipf("Skipping R2 integration test: Access Denied (possibly due to IP/network restrictions or credentials scope: %v)", err)
		}
		require.NoError(t, err, "failed to upload test file to R2")
	}

	// 4. Test Presigned URL Generation
	downloadURL, err := client.GenerateDownloadURL(ctx, targetBucket, testKey, 5*time.Minute)
	require.NoError(t, err, "failed to generate download URL")
	require.NotEmpty(t, downloadURL)
	t.Logf("Generated presigned download URL: %s", downloadURL)

	// 5. Test File Deletion Flow
	t.Logf("Deleting test file from R2: %s", testKey)
	err = client.DeleteFile(ctx, targetBucket, testKey)
	require.NoError(t, err, "failed to delete test file from R2")

	// 6. Test Idempotency of Delete (NoSuchKey error should be swallowed)
	t.Log("Testing idempotency of deletion (deleting already-deleted key)...")
	err = client.DeleteFile(ctx, targetBucket, testKey)
	require.NoError(t, err, "DeleteFile must be idempotent and swallow NoSuchKey errors")
}
