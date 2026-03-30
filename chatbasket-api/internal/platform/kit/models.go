package kit

import (
	"github.com/google/uuid"
)

type UserId struct {
	StringUserId string
	UuidUserId   uuid.UUID
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
