package evm

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cosmossdk.io/log"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	EVM          = "evm"
	engineClient = "engine-client"
	rpcClient    = "rpc-client"
)

type (
	EngineClient ethclient.Client
	RPCClient    ethclient.Client
)

type EVMClient struct {
	config       *EVMConfig
	engineClient *ethclient.Client
	rpcClient    *ethclient.Client
	logger       log.Logger
}

func NewEVMClient(ctx context.Context, cfg *EVMConfig, logger log.Logger) (*EVMClient, error) {
	return &EVMClient{
		config: cfg,
		logger: logger,
	}, nil
}

func (client *EVMClient) Name() string {
	return EVM
}

func (c *EVMClient) Start(
	ctx context.Context,
) error {
	defer func() {
		go c.refreshJWTForRPCClient(ctx, engineClient)
	}()

	c.logger.Info("Connecting to the execution client")

	c.logger.Info("Initializing connection with Ethereum Engine API:", c.config.engineAPIURL)
	if err := c.createConnection(ctx, engineClient); err != nil {
		return nil
	}

	var wg sync.WaitGroup

	wg.Add(2)
	ticker := time.NewTicker(c.config.rpcCheckInterval)
	defer ticker.Stop()
	go func() {
		defer wg.Done()
		for range ticker.C {
			c.logger.Info("Waiting for ethereum engine api to be available: ", c.config.engineAPIURL)
			if err := c.createConnection(ctx, engineClient); err != nil {
				c.logger.Error("failed to create connection to ethereum engine api")
				continue
			}

			break
		}
	}()

	go func() {
		defer wg.Done()
		for range ticker.C {
			c.logger.Info("Waiting for ethereum rpc api to be available: ", c.config.rpcURL)
			if err := c.createConnection(ctx, rpcClient); err != nil {
				c.logger.Error("failed to create connection to ethereum rpc api")
				continue
			}

			break
		}
	}()

	wg.Wait()
	return nil
}

func (c *EVMClient) createConnection(ctx context.Context, clientType string) error {
	var err error

	switch clientType {
	case engineClient:
		var header http.Header
		if header, err = c.buildJWTHeader(); err != nil {
			return err
		}
		client, err := rpc.DialOptions(
			ctx, c.config.engineAPIURL.String(), rpc.WithHeaders(header),
		)
		if err != nil {
			return err
		}
		c.engineClient = ethclient.NewClient(client)

		c.logger.Info("Connected to ethereum engine API: ", c.config.engineAPIURL)

	case rpcClient:
		client, err := rpc.DialContext(ctx, c.config.rpcURL.String())
		if err != nil {
			return err
		}
		c.rpcClient = ethclient.NewClient(client)

		c.logger.Info("Connected to ethereum rpc API: ", c.config.rpcURL)

	default:
		return fmt.Errorf("invalid evm client type")
	}
	return nil
}
