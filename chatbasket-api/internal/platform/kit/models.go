package kit

import (
	"github.com/google/uuid"
)

type UserId struct {
	StringUserId string
	UuidUserId   uuid.UUID
}



// StatusOkay is a generic boolean status and message response.
type StatusOkay struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}

// Documents wraps a list of typed items.
type Documents[T any] struct {
	Documents []T `json:"documents"`
}
