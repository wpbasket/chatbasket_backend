package utils

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/google/uuid"
)

func LoadKeyFromEnvInByte(envVar string) ([]byte, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return nil, fmt.Errorf("missing env var: %s", envVar)
	}
	key, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %v", err)
	}
	return key, nil
}

func LoadKeyFromEnv(envVar string) (string, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return "", fmt.Errorf("missing env var: %s", envVar)
	}
	return val, nil
}

func StringToUUID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, err
	}
	return parsed, nil
}

// AppwriteFileData represents the data needed to construct an appwrite file URI
type AppwriteFileData struct {
	FileId     *string `json:"fileId"`
	FileToken  *string `json:"fileToken"`
	FileSecret *string `json:"fileSecret"`
}

// BuildAvatarURI constructs the avatar URL from AppwriteFileData
// Returns empty string if data is invalid or insufficient tokens
func BuildAvatarURI(ad *AppwriteFileData) *string {
	if ad == nil || ad.FileId == nil || *ad.FileId == "" || ad.FileToken == nil || *ad.FileToken == "" || ad.FileSecret == nil || *ad.FileSecret == "" {
		return nil
	}

	uri := fmt.Sprintf("https://fra.cloud.appwrite.io/v1/storage/buckets/68f1170100025d36bf45/files/%s/view?project=6858ed4d0005c859ea03&token=%s",
		*ad.FileId, *ad.FileSecret)
	return &uri
}

// BuildFileDownloadURL constructs a download URL for chat files
func BuildFileDownloadURL(endpoint, projectID, bucketID string, ad *AppwriteFileData) *string {
	if ad == nil || ad.FileId == nil || *ad.FileId == "" || ad.FileSecret == nil || *ad.FileSecret == "" {
		return nil
	}

	base := endpoint
	// Remove trailing slash
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	// Many users forget to add /v1 to APPWRITE_ENDPOINT
	// If it doesn't end with /v1, add it for these manually constructed URLs
	const v1Suffix = "/v1"
	if len(base) < len(v1Suffix) || base[len(base)-len(v1Suffix):] != v1Suffix {
		base = base + v1Suffix
	}

	uri := fmt.Sprintf("%s/storage/buckets/%s/files/%s/download?project=%s&token=%s",
		base, bucketID, *ad.FileId, projectID, *ad.FileSecret)
	return &uri
}

// BuildFileViewURL constructs a view URL for chat files (better for inline images)
func BuildFileViewURL(endpoint, projectID, bucketID string, ad *AppwriteFileData) *string {
	if ad == nil || ad.FileId == nil || *ad.FileId == "" || ad.FileSecret == nil || *ad.FileSecret == "" {
		return nil
	}

	base := endpoint
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	const v1Suffix = "/v1"
	if len(base) < len(v1Suffix) || base[len(base)-len(v1Suffix):] != v1Suffix {
		base = base + v1Suffix
	}

	uri := fmt.Sprintf("%s/storage/buckets/%s/files/%s/view?project=%s&token=%s",
		base, bucketID, *ad.FileId, projectID, *ad.FileSecret)
	return &uri
}
