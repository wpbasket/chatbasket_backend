package utils

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/appwrite/sdk-for-go/file"
)

// Convert multipart.FileHeader (from Echo) to InputFile
func ConvertToInputFile(fh *multipart.FileHeader) (file.InputFile, error) {
    // Open the multipart file
    opened, err := fh.Open()
    if err != nil {
        return file.InputFile{}, err
    }
    defer opened.Close()
    
    // Create a temporary file with proper extension
    fileExt := filepath.Ext(fh.Filename)
    tempFile, err := os.CreateTemp("", "appwrite_upload_*"+fileExt)
    if err != nil {
        return file.InputFile{}, err
    }
    defer tempFile.Close()
    
    // Copy multipart file content to temp file
    _, err = io.Copy(tempFile, opened)
    if err != nil {
        os.Remove(tempFile.Name())
        return file.InputFile{}, err
    }
    
    // Create InputFile with path
    inputFile := file.InputFile{
        Name: fh.Filename,
        Path: tempFile.Name(),
        Data: nil,
    }
    
    return inputFile, nil
}
