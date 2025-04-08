package evm

import (
	"math/big"
	"net/url"
	"time"
)

const (
	DefaultEngineAPIURL          = "http://localhost:8551"
	DefaultRPCURL                = "http://localhost:8545"
	DefaultRPCJWTRefreshInterval = 20 * time.Second
	DefaultJWTSecretPath         = "./jwt.hex"
	DefaultRPCCheckInterval      = 3 * time.Second
	DefaultEthChainID            = "80087"
	DefaultSocketPath            = "/tmp/ipc/scalerize.sock"
	DefaultCometBFTRPCAddress    = "http://localhost:26657"
)

type EVMConfig struct {
	engineAPIURL          *url.URL
	rpcURL                *url.URL
	jwtSecret             *JWTSecret
	rpcJWTRefreshInterval time.Duration
	rpcCheckInterval      time.Duration
	ethChainID            *big.Int
}

func NewEVMConfig(ethChainID *big.Int, rpcJWTRefreshInterval, rpcCheckInterval time.Duration, engineAPIURL, rpcURL, jwtSecretPath string) (*EVMConfig, error) {
	eu, err := url.Parse(engineAPIURL)
	if err != nil {
		return nil, err
	}

	ru, err := url.Parse(rpcURL)
	if err != nil {
		return nil, err
	}

	secret, err := getJWTFromPath(jwtSecretPath)
	if err != nil {
		return nil, err
	}

	return &EVMConfig{
		engineAPIURL:          eu,
		rpcURL:                ru,
		jwtSecret:             &secret,
		rpcJWTRefreshInterval: rpcJWTRefreshInterval,
		rpcCheckInterval:      rpcCheckInterval,
		ethChainID:            ethChainID,
	}, nil
}

func (c *EVMConfig) EngineAPIURL() *url.URL {
	return c.engineAPIURL
}

func (c *EVMConfig) RPCURL() *url.URL {
	return c.rpcURL
}

func (c *EVMConfig) JWTSecret() *JWTSecret {
	return c.jwtSecret
}

func (c *EVMConfig) RPCJWTRefreshInterval() time.Duration {
	return c.rpcJWTRefreshInterval
}

func (c *EVMConfig) RPCCheckInterval() time.Duration {
	return c.rpcCheckInterval
}

func (c *EVMConfig) ETHChainID() *big.Int {
	return c.ethChainID
}
