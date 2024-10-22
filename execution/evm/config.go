package evm

import (
	"net/url"
	"time"
)

const (
	DefaultEngineAPIURL          = "http://localhost:8551"
	DefaultRPCURL                = "http://localhost:8545"
	DefaultRPCJWTRefreshInterval = 20 * time.Second
	DefaultJWTSecretPath         = "./jwt.hex"
	DefaultRPCCheckInterval      = 3 * time.Second
)

type EVMConfig struct {
	engineAPIURL          *url.URL
	rpcURL                *url.URL
	jwtSecret             *JWTSecret
	rpcJWTRefreshInterval time.Duration
	rpcCheckInterval      time.Duration
}

func NewEVMConfig(rpcJWTRefreshInterval, rpcCheckInterval time.Duration, engineAPIURL, rpcURL, jwtSecretPath string) (*EVMConfig, error) {
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
	}, nil
}
