package db

import (
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

func NewCosmosClient(cfg *CosmosConfig) (*azcosmos.Client, error) {
	// Options from reference: EnableContentResponseOnWrite
	clientOptions := azcosmos.ClientOptions{
		EnableContentResponseOnWrite: true,
	}

	client, err := azcosmos.NewClientFromConnectionString(cfg.ConnectionString, &clientOptions)
	if err != nil {
		return nil, err
	}

	return client, nil
}
