package kit

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/appwrite/sdk-for-go/file"
	"github.com/google/uuid"
)

// ported from utils/baseUtils.go
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

// ported from utils/baseUtils.go
func LoadKeyFromEnv(envVar string) (string, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return "", fmt.Errorf("missing env var: %s", envVar)
	}
	return val, nil
}

// ported from utils/baseUtils.go
func StringToUUID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, err
	}
	return parsed, nil
}

// AppwriteFileData represents the data needed to construct an appwrite file URI, ported from utils/baseUtils.go
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

// BuildFileDownloadURL constructs a download URL for chat files, ported from utils/baseUtils.go
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

// BuildFileViewURL constructs a view URL for chat files (better for inline images), ported from utils/baseUtils.go
func BuildFileViewURL(endpoint, projectID, bucketID string, ad *AppwriteFileData) *string {
	if ad == nil || ad.FileId == nil || *ad.FileId == "" || ad.FileSecret == nil || *ad.FileSecret == "" {
		return nil
	}

	base := endpoint
	// Remove trailing slash
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

// Convert multipart.FileHeader (from Echo) to InputFile, ported from utils/toInputFileUtils.go
func ConvertToInputFile(fh *multipart.FileHeader) (file.InputFile, *ApiError) {
	// Open the multipart file
	opened, err := fh.Open()
	if err != nil {
		return file.InputFile{}, &ApiError{
			Code:    500,
			Message: "Failed to open multipart file: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	defer opened.Close()

	// Create a temporary file with proper extension
	fileExt := filepath.Ext(fh.Filename)
	tempFile, err := os.CreateTemp("", "appwrite_upload_*"+fileExt)
	if err != nil {
		return file.InputFile{}, &ApiError{
			Code:    500,
			Message: "Failed to create temporary file: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	defer tempFile.Close()

	// Copy multipart file content to temp file
	_, err = io.Copy(tempFile, opened)
	if err != nil {
		// Ensure temp file is cleaned up on error
		if removeErr := os.Remove(tempFile.Name()); removeErr != nil {
			// Log the cleanup error but return the original error
			// In production, you might want to use a proper logger here
		}
		return file.InputFile{}, &ApiError{
			Code:    500,
			Message: "Failed to copy file content: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Create InputFile with path
	inputFile := file.InputFile{
		Name: fh.Filename,
		Path: tempFile.Name(),
		Data: nil,
	}

	return inputFile, nil
}
