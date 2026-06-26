package clients

import (
	appconfig "chatbasket-api/internal/platform/config"
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// R2Client wraps the S3 client configured for Cloudflare R2.
type R2Client struct {
	S3Client         *s3.Client
	PresignClient    *s3.PresignClient
	ChatFilesBucket  string
	ProfilePicBucket string
}

// NewR2Client initializes a new S3 client configured for Cloudflare R2.
func NewR2Client(cfg *appconfig.R2AccountConfig) (*R2Client, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("R2 account '%s' configuration is incomplete", cfg.Name)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS SDK config for R2 account '%s': %w", cfg.Name, err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID))
	})

	presignClient := s3.NewPresignClient(s3Client)

	return &R2Client{
		S3Client:         s3Client,
		PresignClient:    presignClient,
		ChatFilesBucket:  cfg.ChatFilesBucket,
		ProfilePicBucket: cfg.ProfilePicBucket,
	}, nil
}

// UploadFile uploads an io.Reader stream to R2.
func (c *R2Client) UploadFile(ctx context.Context, bucket, key string, body io.Reader, contentType string) error {
	targetBucket := bucket
	if targetBucket == "" {
		targetBucket = c.ChatFilesBucket
	}
	_, err := c.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(targetBucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to R2: %w", err)
	}
	return nil
}

// DeleteFile deletes an object from R2. IDEMPOTENT: "NoSuchKey", "NotFound",
// and "NoSuchBucket" errors are treated as success (object already gone).
// Verified against real R2 via lifecycle test.
func (c *R2Client) DeleteFile(ctx context.Context, bucket, key string) error {
	targetBucket := bucket
	if targetBucket == "" {
		targetBucket = c.ChatFilesBucket
	}
	_, err := c.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(targetBucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return nil
		}
	}
	return fmt.Errorf("failed to delete file from R2: %w", err)
}

// GenerateDownloadURL generates a presigned GET URL.
func (c *R2Client) GenerateDownloadURL(ctx context.Context, bucket, key string, lifetime time.Duration) (string, error) {
	targetBucket := bucket
	if targetBucket == "" {
		targetBucket = c.ChatFilesBucket
	}
	req, err := c.PresignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(targetBucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = lifetime
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned GET URL: %w", err)
	}
	return req.URL, nil
}

// GenerateUploadURL generates a presigned PUT URL.
func (c *R2Client) GenerateUploadURL(ctx context.Context, bucket, key string, lifetime time.Duration) (string, error) {
	targetBucket := bucket
	if targetBucket == "" {
		targetBucket = c.ChatFilesBucket
	}
	req, err := c.PresignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(targetBucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = lifetime
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned PUT URL: %w", err)
	}
	return req.URL, nil
}

// ChatBucket returns the chat files bucket name.
func (c *R2Client) ChatBucket() string { return c.ChatFilesBucket }

// ProfileBucket returns the profile pictures bucket name.
func (c *R2Client) ProfileBucket() string { return c.ProfilePicBucket }

// ──────────────────────────────────────────────────────────────────────────────
// R2ClientPool — thread-safe pool with round-robin and safe fallback
// ──────────────────────────────────────────────────────────────────────────────

// R2ClientPool maintains a map of R2 clients keyed by account name and supports
// round-robin distribution for separate chat and profile buckets. All file IDs are
// prefixed with the account name (e.g., "chat-01:uuid") for routing.
//
// Example multi-bucket DEV configuration:
// R2_ACCOUNTS_JSON='[
//   {"name":"chat-files-01", "account_id":"...", "access_key_id":"...", "secret_access_key":"...", "chat_files_bucket":"dev-chat-files-01", "profile_pic_bucket":""},
//   {"name":"profile-pic-01", "account_id":"...", "access_key_id":"...", "secret_access_key":"...", "chat_files_bucket":"", "profile_pic_bucket":"dev-profile-pic-01"}
// ]'
// R2_PRIMARY_CHAT_ACCOUNT=chat-files-01
// R2_PRIMARY_PROFILE_ACCOUNT=profile-pic-01
//
// Example multi-bucket PROD configuration:
// R2_ACCOUNTS_JSON='[
//   {"name":"chat-files-01", "account_id":"...", "access_key_id":"...", "secret_access_key":"...", "chat_files_bucket":"chat-files-01", "profile_pic_bucket":""},
//   {"name":"profile-pic-01", "account_id":"...", "access_key_id":"...", "secret_access_key":"...", "chat_files_bucket":"", "profile_pic_bucket":"profile-pic-01"}
// ]'
// R2_PRIMARY_CHAT_ACCOUNT=chat-files-01
// R2_PRIMARY_PROFILE_ACCOUNT=profile-pic-01
type R2ClientPool struct {
	clients               map[string]*R2Client
	activeChatAccounts    []string
	activeProfileAccounts []string
	primaryChatAccount    string
	primaryProfileAccount string
	chatCounter           uint64
	profileCounter        uint64
}

// NewR2ClientPool initializes a pool from a list of R2 account configs.
// Returns error if no accounts configured (fatal at startup).
func NewR2ClientPool(cfg *appconfig.R2PoolConfig) (*R2ClientPool, error) {
	if len(cfg.Accounts) == 0 {
		return nil, fmt.Errorf("R2 client pool requires at least one configured account")
	}
	clients := make(map[string]*R2Client, len(cfg.Accounts))
	chatNames := make([]string, 0, len(cfg.Accounts))
	profileNames := make([]string, 0, len(cfg.Accounts))
	for i := range cfg.Accounts {
		acc := &cfg.Accounts[i]
		client, err := NewR2Client(acc)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize R2 account '%s': %w", acc.Name, err)
		}
		clients[acc.Name] = client
		if acc.ChatFilesBucket != "" {
			chatNames = append(chatNames, acc.Name)
		}
		if acc.ProfilePicBucket != "" {
			profileNames = append(profileNames, acc.Name)
		}
	}
	if _, ok := clients[cfg.PrimaryChatAccount]; !ok {
		return nil, fmt.Errorf("primary chat account '%s' not found in configured accounts", cfg.PrimaryChatAccount)
	}
	if _, ok := clients[cfg.PrimaryProfileAccount]; !ok {
		return nil, fmt.Errorf("primary profile account '%s' not found in configured accounts", cfg.PrimaryProfileAccount)
	}
	return &R2ClientPool{
		clients:               clients,
		activeChatAccounts:    chatNames,
		activeProfileAccounts: profileNames,
		primaryChatAccount:    cfg.PrimaryChatAccount,
		primaryProfileAccount: cfg.PrimaryProfileAccount,
		chatCounter:           0,
		profileCounter:        0,
	}, nil
}

// SetActiveAccounts updates the list of accounts used for round-robin.
// Removed accounts will not receive new uploads but remain in the map for
// safe retrieval/deletion of existing files (per spec §3.B).
func (p *R2ClientPool) SetActiveAccounts(names []string) {
	if len(names) == 0 {
		p.activeChatAccounts = p.activeChatAccounts[:0]
		p.activeProfileAccounts = p.activeProfileAccounts[:0]
		for name, client := range p.clients {
			if client.ChatFilesBucket != "" {
				p.activeChatAccounts = append(p.activeChatAccounts, name)
			}
			if client.ProfilePicBucket != "" {
				p.activeProfileAccounts = append(p.activeProfileAccounts, name)
			}
		}
		return
	}
	chatFiltered := make([]string, 0, len(names))
	profileFiltered := make([]string, 0, len(names))
	for _, n := range names {
		if client, ok := p.clients[n]; ok {
			if client.ChatFilesBucket != "" {
				chatFiltered = append(chatFiltered, n)
			}
			if client.ProfilePicBucket != "" {
				profileFiltered = append(profileFiltered, n)
			}
		}
	}
	if len(chatFiltered) > 0 {
		p.activeChatAccounts = chatFiltered
	}
	if len(profileFiltered) > 0 {
		p.activeProfileAccounts = profileFiltered
	}
}

// NextAccount returns the next account name for new uploads via atomic round-robin.
func (p *R2ClientPool) NextChatAccount() string {
	if len(p.activeChatAccounts) == 0 {
		return p.primaryChatAccount
	}
	idx := atomic.AddUint64(&p.chatCounter, 1) - 1
	return p.activeChatAccounts[int(idx%uint64(len(p.activeChatAccounts)))]
}

// NextProfileAccount returns the next account name for new profile uploads via atomic round-robin.
func (p *R2ClientPool) NextProfileAccount() string {
	if len(p.activeProfileAccounts) == 0 {
		return p.primaryProfileAccount
	}
	idx := atomic.AddUint64(&p.profileCounter, 1) - 1
	return p.activeProfileAccounts[int(idx%uint64(len(p.activeProfileAccounts)))]
}

// GetClient returns the R2 client for a given file ID, routing by account prefix.
// Defaults to chat primary if missing or un-prefixed.
func (p *R2ClientPool) GetClient(fileID string) *R2Client {
	accountName, _ := ParseFilePrefix(fileID)
	if accountName != "" {
		if client, ok := p.clients[accountName]; ok {
			return client
		}
		return p.clients[p.primaryChatAccount]
	}
	return p.clients[p.primaryChatAccount]
}

// GetClientByAccount returns the R2 client for a given account name directly.
// Falls back to the primary client if the account name is empty or retired (per spec §3.E).
func (p *R2ClientPool) GetClientByAccount(accountName string) *R2Client {
	if accountName != "" {
		if client, ok := p.clients[accountName]; ok {
			return client
		}
	}
	return p.clients[p.primaryChatAccount]
}


// PrimaryChatClient returns the R2 client for the configured primary chat account.
func (p *R2ClientPool) PrimaryChatClient() *R2Client {
	return p.clients[p.primaryChatAccount]
}

// PrimaryProfileClient returns the R2 client for the configured primary profile account.
func (p *R2ClientPool) PrimaryProfileClient() *R2Client {
	return p.clients[p.primaryProfileAccount]
}

// HasClient returns true if the given account name exists in the pool.
func (p *R2ClientPool) HasClient(accountName string) bool {
	_, ok := p.clients[accountName]
	return ok
}

// ParseFilePrefix splits a prefixed file ID into account name and object UUID.
// Returns ("", rawID) if no prefix present.
func ParseFilePrefix(fileID string) (accountName, objectID string) {
	for i := 0; i < len(fileID); i++ {
		if fileID[i] == ':' {
			return fileID[:i], fileID[i+1:]
		}
	}
	return "", fileID
}

// BuildFileID constructs a prefixed file ID.
func BuildFileID(accountName, objectID string) string {
	return accountName + ":" + objectID
}
