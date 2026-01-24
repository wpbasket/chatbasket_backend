package db

import (
	"errors"
	"os"
)

type CosmosConfig struct {
	ConnectionString string
	Database         string
	Container        string
}

func LoadCosmosConfig() (*CosmosConfig, error) {
	connStr := os.Getenv("COSMOS_CONNECTION_STRING")
	if connStr == "" {
		return nil, errors.New("COSMOS_CONNECTION_STRING environment variable is not set")
	}

	database := os.Getenv("COSMOS_DATABASE")
	if database == "" {
		return nil, errors.New("COSMOS_DATABASE environment variable is not set")
	}

	container := os.Getenv("COSMOS_CONTAINER")
	if container == "" {
		return nil, errors.New("COSMOS_CONTAINER environment variable is not set")
	}

	return &CosmosConfig{
		ConnectionString: connStr,
		Database:         database,
		Container:        container,
	}, nil
}
