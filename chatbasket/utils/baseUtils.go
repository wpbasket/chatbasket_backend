package utils

import (
	"encoding/base64"
	"os"

	"github.com/labstack/echo/v4"
)

func LoadKeyFromEnvInByte(envVar string) ([]byte, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return nil, echo.NewHTTPError(500, "missing env var: "+envVar)
	}
	key, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, echo.NewHTTPError(500, "failed to decode base64 key: "+err.Error())
	}
	return key, nil
}

func LoadKeyFromEnv(envVar string) (string, error) {
	val := os.Getenv(envVar)
	if val == "" {
		return "", echo.NewHTTPError(500, "missing env var: "+envVar)
	}
	return val, nil
}
