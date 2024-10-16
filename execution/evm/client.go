package evm

import "github.com/ethereum/go-ethereum/ethclient"

const (
	EVM = "evm"
)

type EVMClient struct {
	Client *ethclient.Client
}

func NewClient(c *ethclient.Client) *EVMClient {
	return &EVMClient{
		Client: c,
	}
}

func (client *EVMClient) Name() string {
	return EVM
}
