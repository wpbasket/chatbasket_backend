package clients

import (
	"chatbasket-api/internal/platform/config"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

// ConnectCosmos creates a new Cosmos DB client, ported from db/cosmos_client.go
func ConnectCosmos(cfg *config.CosmosConfig) (*azcosmos.Client, error) {
	clientOptions := azcosmos.ClientOptions{
		EnableContentResponseOnWrite: true,
	}

	client, err := azcosmos.NewClientFromConnectionString(cfg.ConnectionString, &clientOptions)
	if err != nil {
		return nil, err
	}

	return client, nil
}

