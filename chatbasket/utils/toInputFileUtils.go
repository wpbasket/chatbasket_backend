package utils

import (
	"chatbasket/model"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/appwrite/sdk-for-go/file"
)

// Convert multipart.FileHeader (from Echo) to InputFile
func ConvertToInputFile(fh *multipart.FileHeader) (file.InputFile, *model.ApiError) {
    // Open the multipart file
    opened, err := fh.Open()
    if err != nil {
        return file.InputFile{}, &model.ApiError{
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
        return file.InputFile{}, &model.ApiError{
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
        return file.InputFile{}, &model.ApiError{
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
