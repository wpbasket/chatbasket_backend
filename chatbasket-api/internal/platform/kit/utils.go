package kit

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/appwrite/sdk-for-go/file"
	"github.com/appwrite/sdk-for-go/tokens"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// DerefTime safely dereferences a *time.Time to time.Time, returning Time{} if nil.
func DerefTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

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
	if ad == nil || ad.FileId == nil || *ad.FileId == "" || ad.FileSecret == nil || *ad.FileSecret == "" {
		return nil
	}

	uri := fmt.Sprintf("https://fra.cloud.appwrite.io/v1/storage/buckets/68f1170100025d36bf45/files/%s/view?project=6858ed4d0005c859ea03&token=%s",
		*ad.FileId, *ad.FileSecret)
	return &uri
}

// RefreshFileData represents updated token metadata
type RefreshFileData struct {
	TokenID     string
	TokenSecret string
	TokenExpiry time.Time
}

// EnsureFreshFileTokens checks if Appwrite file tokens need refresh and regenerates them if needed.

// Refresh is triggered when:
//   - tokenExpiry is zero (NULL in DB) OR tokenID is empty OR tokenSecret is empty
//   - tokenExpiry has already expired
//
// When refresh is triggered: lists all existing tokens for the file, deletes them one by one, then creates a new token.
// This ensures Appwrite does not accumulate orphaned tokens.
//
// Returns (refreshedData, needsUpdate, error).
func EnsureFreshFileTokens(
	fileID *string,
	tokenID *string,
	tokenSecret *string,
	tokenExpiry time.Time,
	appwriteTokens *tokens.Tokens,
	bucketID string,
) (*RefreshFileData, bool, error) {
	if fileID == nil || *fileID == "" {
		return nil, false, nil
	}

	now := time.Now().UTC()

	// Determine if refresh is needed
	needsRefresh := false

	// Refresh if any credential is missing (zero/empty)
	if tokenExpiry.IsZero() || (tokenID == nil || *tokenID == "") || (tokenSecret == nil || *tokenSecret == "") {
		needsRefresh = true
	} else if !tokenExpiry.UTC().After(now) {
		// Refresh if expiry has passed
		needsRefresh = true
	}

	if !needsRefresh {
		return nil, false, nil
	}

	// ——— Refresh triggered: delete all existing tokens for this file ——————————————————————
	tokenList, err := appwriteTokens.List(bucketID, *fileID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list existing tokens for file %s: %w", *fileID, err)
	}
	if tokenList.Total > 0 {
		for _, tok := range tokenList.Tokens {
			if _, delErr := appwriteTokens.Delete(tok.Id); delErr != nil {
				log.Printf("[EnsureFreshFileTokens] warning: failed to delete token %s for file %s: %v", tok.Id, *fileID, delErr)
			}
		}
	}

	// ——— Create new token —————————————————————————————————————————————————————————————————————
	exp := now.AddDate(1, 0, 0).Format("2006-01-02 15:04:05")
	newTok, err := appwriteTokens.CreateFileToken(
		bucketID,
		*fileID,
		appwriteTokens.WithCreateFileTokenExpire(exp),
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create Appwrite file token: %w", err)
	}

	tokTime, err := time.Parse(time.RFC3339, newTok.Expire)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse Appwrite expire time: %w", err)
	}

	return &RefreshFileData{
		TokenID:     newTok.Id,
		TokenSecret: newTok.Secret,
		TokenExpiry: tokTime,
	}, true, nil
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
func ConvertToInputFile(fh *multipart.FileHeader) (file.InputFile, error) {
	// Open the multipart file
	opened, err := fh.Open()
	if err != nil {
		return file.InputFile{}, fmt.Errorf("failed to open multipart file: %v", err)
	}
	defer opened.Close()

	// Create a temporary file with proper extension
	fileExt := filepath.Ext(fh.Filename)
	tempFile, err := os.CreateTemp("", "appwrite_upload_*"+fileExt)
	if err != nil {
		return file.InputFile{}, fmt.Errorf("failed to create temporary file: %v", err)
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
		return file.InputFile{}, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Create InputFile with path
	inputFile := file.InputFile{
		Name: fh.Filename,
		Path: tempFile.Name(),
		Data: nil,
	}

	return inputFile, nil
}

// ExtractUserID extracts UserId from Echo context (set by auth middleware)
func ExtractUserID(c *echo.Context) (UserId, error) {
	userId, okStr := (*c).Get("userId").(string)
	uuidUserId, okUUID := (*c).Get("uuidUserId").(uuid.UUID)
	if !okStr || userId == "" || !okUUID {
		return UserId{}, NewError(http.StatusUnauthorized, "unauthorized", "User id is missing or invalid")
	}
	return UserId{
		StringUserId: userId,
		UuidUserId:   uuidUserId,
	}, nil
}
