package services

import (
)

// GlobalService holds all global service dependencies that modules need.
type GlobalService struct {
	// Add more services here as needed in the future
	// e.g., RedisClient, S3Client, EmailService, etc.
}

// NewGlobalService creates a new GlobalService instance with all dependencies.
func NewGlobalService() *GlobalService {
	return &GlobalService{}
}
