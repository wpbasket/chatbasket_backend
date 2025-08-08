package utils

import (
    "io"
    "mime/multipart"
    "github.com/appwrite/sdk-for-go/file"
)

// Convert multipart.FileHeader (from Echo) to InputFile
func ConvertToInputFile(fh *multipart.FileHeader) (file.InputFile, error) {
    opened, err := fh.Open()
    if err != nil {
        return file.InputFile{}, err
    }
    defer opened.Close()
    
    data := make([]byte, fh.Size)
    _, err = io.ReadFull(opened, data)
    if err != nil {
        return file.InputFile{}, err
    }
    
    return file.InputFile{
        Name: fh.Filename,
        Data: data,
    }, nil
}
