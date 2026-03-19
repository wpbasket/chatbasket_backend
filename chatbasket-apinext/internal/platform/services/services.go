package services

import (
	"chatbasket-apinext/internal/store/postgresgen"
)

// GlobalService holds all global service dependencies that modules need.
type GlobalService struct {
	PostgresQuerier postgresgen.Querier  // For regular queries (interface for testing)
	PostgresQueries *postgresgen.Queries // For transactions (concrete type with WithTx)
	// Add more services here as needed in the future
	// e.g., RedisClient, S3Client, EmailService, etc.
}

// NewGlobalService creates a new GlobalService instance with all dependencies.
func NewGlobalService(postgresQueries *postgresgen.Queries) *GlobalService {
	return &GlobalService{
		PostgresQuerier: postgresQueries, // Automatically converts to interface
		PostgresQueries: postgresQueries, // Keep concrete type for WithTx
	}
}
