package model

type Documents[T any] struct {
    Documents []T `json:"documents"`
}