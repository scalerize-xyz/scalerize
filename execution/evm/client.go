package evm

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	cometbfthttp "github.com/cometbft/cometbft/rpc/client/http"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	cosmossdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	EVM          = "evm"
	engineClient = "engine-client"
	rpcClient    = "rpc-client"
)

type SyncStatus struct {
	syncing bool
}

type EVMClient struct {
	ctx             context.Context
	app             *baseapp.BaseApp
	config          *EVMConfig
	engineClient    *ethclient.Client
	rpcClient       *ethclient.Client
	cosmosRPCClient cosmossdkclient.CometRPC
	logger          log.Logger
	syncStatus      *SyncStatus
}

func NewEVMClient(ctx context.Context, cfg *EVMConfig, logger log.Logger) (*EVMClient, error) {
	return &EVMClient{
		ctx:    ctx,
		config: cfg,
		logger: logger,
	}, nil
}

func (client *EVMClient) Name() string {
	return EVM
}

func (client *EVMClient) SetApp(app *baseapp.BaseApp) {
	client.app = app
}

func (evmClient *EVMClient) SyncingStatus() (bool, error) {
	status, err := evmClient.cosmosRPCClient.Status(context.Background())
	if err != nil {
		return false, err
	}

	return status.SyncInfo.CatchingUp, nil
}

func (client *EVMClient) SetCosmosRPCClient(cometBFTRPCAddress string) {
	maxRetries := 10
	retryDelay := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		cometBFTClient, err := cometbfthttp.New(cometBFTRPCAddress, "/websocket")
		if err == nil {
			interfaceRegistry := types.NewInterfaceRegistry()
			marshaler := codec.NewProtoCodec(interfaceRegistry)
			clientCtx := cosmossdkclient.Context{}.
				WithClient(cometBFTClient).
				WithCodec(marshaler).
				WithInterfaceRegistry(interfaceRegistry)

			client.cosmosRPCClient = clientCtx.Client
			return
		}

		time.Sleep(retryDelay)
	}

	client.logger.Error("FAILED TO CONNECT TO COMSOS RPC CLIENT")
}

func (c *EVMClient) Start(ctx context.Context, ensureClientCreatedCh chan bool) error {
	var (
		wg     sync.WaitGroup
		ticker = time.NewTicker(c.config.RPCCheckInterval())
	)

	defer func() {
		ticker.Stop()
		go c.refreshJWTForClient(ctx, engineClient)
	}()

	c.logger.Info("Connecting to the execution client")
	wg.Add(2)

	c.logger.Info("Initializing connection with Ethereum Engine API: " + c.config.EngineAPIURL().String())
	go func() {
		defer wg.Done()
		for range ticker.C {
			c.logger.Info("Waiting for ethereum engine api to be available: " + c.config.EngineAPIURL().String())
			if err := c.connect(engineClient); err != nil {
				c.logger.Error("failed to create connection to ethereum engine api")
				continue
			}

			break
		}
	}()

	c.logger.Info("Initializing connection with Ethereum RPC API: " + c.config.RPCURL().String())
	go func() {
		defer wg.Done()
		for range ticker.C {
			c.logger.Info("Waiting for ethereum rpc api to be available: " + c.config.RPCURL().String())
			if err := c.connect(rpcClient); err != nil {
				c.logger.Error("failed to create connection to ethereum rpc api")
				continue
			}

			break
		}
	}()

	wg.Wait()
	ensureClientCreatedCh <- true
	return nil
}

func (c *EVMClient) connect(clientType string) error {
	if err := c.dialRPCCLient(clientType); err != nil {
		return err
	}

	switch clientType {
	case engineClient:
		if _, err := c.ExchangeCapabilities(ScalerizeSupportedCapabilities()); err != nil {
			c.logger.Error("failed to exchange capabilities: " + err.Error())
			return err
		}

		c.logger.Info("Connected to ethereum engine API: " + c.config.EngineAPIURL().String())

	case rpcClient:
		chainID, err := c.rpcClient.NetworkID(context.Background())
		if err != nil {
			c.logger.Error("failed to get eth network id: " + err.Error())
			return err
		}

		if chainID.Cmp(c.config.ETHChainID()) != 0 {
			c.logger.Error("eth chain ID specified not equal to the actual chain ID")
			return fmt.Errorf("chainID do not match for the eth client with what specified in scalerize config")
		}

		c.logger.Info("Connected to ethereum RPC API: " + c.config.RPCURL().String())

	default:
		return fmt.Errorf("invalid evm client type")
	}

	return nil
}

func (c *EVMClient) dialRPCCLient(clientType string) error {
	var err error

	switch clientType {
	case engineClient:
		var header http.Header
		if header, err = c.buildJWTHeader(); err != nil {
			return err
		}
		client, err := rpc.DialOptions(
			c.ctx, c.config.EngineAPIURL().String(), rpc.WithHeaders(header),
		)
		if err != nil {
			return err
		}
		c.engineClient = ethclient.NewClient(client)

	case rpcClient:
		client, err := rpc.DialContext(c.ctx, c.config.RPCURL().String())
		if err != nil {
			return err
		}
		c.rpcClient = ethclient.NewClient(client)

	default:
		return fmt.Errorf("invalid evm client type")
	}
	return nil
}
