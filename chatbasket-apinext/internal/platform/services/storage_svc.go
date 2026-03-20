package services

import (
	"chatbasket-apinext/internal/platform/clients"
	"chatbasket-apinext/internal/platform/kit"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/appwrite/sdk-for-go/query"
)

// UploadOptions controls optional behaviors for file upload.
type UploadOptions struct {
	// DeleteExisting will check if a file with the same ID exists and delete it before upload.
	DeleteExisting bool
	// GenerateTokens will create a personal file token after upload.
	GenerateTokens bool
}

// UploadResult contains the outcome of an upload.
type UploadResult struct {
	FileId       string
	Name         string
	Expire       string
	TokenIDs     []string // [personalTokenId]
	TokenSecrets []string // [personalTokenSecret]
}

// UploadFileFromMultipart uploads a file to Appwrite Storage from a multipart.FileHeader.
// It will handle temporary file creation/cleanup and (optionally) delete existing files
// and create file access tokens.
func UploadFileFromMultipart(
	appwriteStorage *clients.AppwriteStorageService,
	bucketId string,
	fileId string,
	fh *multipart.FileHeader,
	opts UploadOptions,
) (*UploadResult, error) {
	inputFile, err := kit.ConvertToInputFile(fh)
	if err != nil {
		return nil, err
	}

	// Clean up temp file after upload
	defer func() {
		if inputFile.Path != "" {
			os.Remove(inputFile.Path)
		}
	}()

	// Optionally delete existing file with same ID
	if opts.DeleteExisting {

		// delete file tokens
		tok, err := appwriteStorage.Tokens.List(bucketId, fileId)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to list tokens: "+err.Error())
		}
		if tok.Total > 0 {
			for _, tokens := range tok.Tokens {
				_, err := appwriteStorage.Tokens.Delete(tokens.Id)
				if err != nil {
					return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete token: "+err.Error())
				}
			}
		}

		// delete file
		listFilesRes, err := appwriteStorage.Storage.ListFiles(
			bucketId,
			appwriteStorage.Storage.WithListFilesQueries([]string{
				query.Equal("$id", fileId),
			}),
		)
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to list existing file: "+err.Error())
		}
		if listFilesRes.Total == 1 {
			if _, err := appwriteStorage.Storage.DeleteFile(bucketId, fileId); err != nil {
				return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to delete existing file: "+err.Error())
			}
		}
	}

	uploadRes, err := appwriteStorage.Storage.CreateFile(bucketId, fileId, inputFile)
	if err != nil {
		return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to upload file: "+err.Error())
	}

	result := &UploadResult{
		FileId: uploadRes.Id,
		Name:   uploadRes.Name,
	}

	if opts.GenerateTokens {
		exp := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
		personalToken, err := appwriteStorage.Tokens.CreateFileToken(bucketId, fileId, appwriteStorage.Tokens.WithCreateFileTokenExpire(exp))
		if err != nil {
			return nil, kit.NewError(http.StatusInternalServerError, "internal_server_error", "Failed to create personal token: "+err.Error())
		}
		result.TokenIDs = []string{personalToken.Id}
		result.TokenSecrets = []string{personalToken.Secret}
		result.Expire = personalToken.Expire
	}

	return result, nil
}
