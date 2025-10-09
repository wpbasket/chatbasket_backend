package services

import (
	"chatbasket/model"
	"chatbasket/utils"
	"mime/multipart"
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
func (gs *GlobalService) UploadFileFromMultipart(
	bucketId string,
	fileId string,
	fh *multipart.FileHeader,
	opts UploadOptions,
) (*UploadResult, *model.ApiError) {
	inputFile, apiErr := utils.ConvertToInputFile(fh)
	if apiErr != nil {
		return nil, apiErr
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
		tok, err := gs.Appwrite.Tokens.List(bucketId, fileId)
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to list tokens: " + err.Error(), Type: "internal_server_error"}
		}
		if tok.Total > 0 {
			for _, tokens := range tok.Tokens {
				_, err := gs.Appwrite.Tokens.Delete(tokens.Id)
				if err != nil {
					return nil, &model.ApiError{Code: 500, Message: "Failed to delete token: " + err.Error(), Type: "internal_server_error"}
				}
			}
		}

		// delete file
		listFilesRes, err := gs.Appwrite.Storage.ListFiles(
			bucketId,
			gs.Appwrite.Storage.WithListFilesQueries([]string{
				query.Equal("$id", fileId),
			}),
		)
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to list existing file: " + err.Error(), Type: "internal_server_error"}
		}
		if listFilesRes.Total == 1 {
			if _, err := gs.Appwrite.Storage.DeleteFile(bucketId, fileId); err != nil {
				return nil, &model.ApiError{Code: 500, Message: "Failed to delete existing file: " + err.Error(), Type: "internal_server_error"}
			}
		}
	}

	uploadRes, err := gs.Appwrite.Storage.CreateFile(bucketId, fileId, inputFile)
	if err != nil {
		return nil, &model.ApiError{Code: 500, Message: "Failed to upload file: " + err.Error(), Type: "internal_server_error"}
	}

	result := &UploadResult{
		FileId: uploadRes.Id,
		Name:   uploadRes.Name,
	}

	if opts.GenerateTokens {
		exp := time.Now().AddDate(1, 0, 0).Format("2006-01-02T15:04:05.000Z")
		personalToken, err := gs.Appwrite.Tokens.CreateFileToken(bucketId, fileId, gs.Appwrite.Tokens.WithCreateFileTokenExpire(exp))
		if err != nil {
			return nil, &model.ApiError{Code: 500, Message: "Failed to create personal token: " + err.Error(), Type: "internal_server_error"}
		}
		result.TokenIDs = []string{personalToken.Id}
		result.TokenSecrets = []string{personalToken.Secret}
		result.Expire = personalToken.Expire
	}

	return result, nil
}
