package model

import (
	"github.com/google/uuid"
)

type Documents[T any] struct {
	Documents []T `json:"documents"`
}

type UserId struct {
	StringUserId     string     
	UuidUserId       uuid.UUID  
}