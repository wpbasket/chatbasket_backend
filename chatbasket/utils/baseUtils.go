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
	FileId     *string   `json:"fileId"`
	FileToken  *string   `json:"fileToken"`
	FileSecret *string   `json:"fileSecret"`
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