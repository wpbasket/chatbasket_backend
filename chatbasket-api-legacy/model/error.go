package model

type SessionError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

type ApiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// AppError is for internal/non-API errors
type AppError struct {
	Type    string
	Message string
}
