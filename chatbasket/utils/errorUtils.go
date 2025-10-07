package utils

import (
	"github.com/labstack/echo/v4"
)

// GetStatusCodeFromError extracts the HTTP status code from an error.
func GetStatusCodeFromError(err error) int{
	he := err.(*echo.HTTPError)
	return he.Code
}
