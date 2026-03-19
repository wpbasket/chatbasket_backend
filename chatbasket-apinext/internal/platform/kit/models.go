package kit

import (
	"github.com/google/uuid"
)

type UserId struct {
	StringUserId string    `json:"userId"`
	UuidUserId   uuid.UUID `json:"uuidUserId"`
}

// ported from chatbasket-api/model/error.go
type ApiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// AppError is for internal/non-API errors, ported from chatbasket-api/model/error.go
type AppError struct {
	Type    string
	Message string
}

// ported from chatbasket-api/model/user.go
type StatusOkay struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}

// ported from chatbasket-api/model/base.go
type Documents[T any] struct {
	Documents []T `json:"documents"`
}
