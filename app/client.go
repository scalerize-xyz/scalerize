package app

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"github.com/aerius-labs/scalerize/app/params"
	evmexec "github.com/aerius-labs/scalerize/execution/evm"
	"github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

type ExecutionClient interface {
	Name() string
	Start(ctx context.Context, ready chan bool) error
	SetApp(*baseapp.BaseApp)
	ABCIHandler
}

func (app *ScalerizeApp) NewClient(ctx context.Context, appOpts servertypes.AppOptions, logger log.Logger) (ExecutionClient, error) {
	clientType := appOpts.Get(params.FlagExecutionClientType).(string)
	switch clientType {
	case evmexec.EVM:
		app.executionTablesInfo = map[uint8]tableInfo{
			HashedAccountsTableCode: {
				DupSorted: false,
				KeyBytes:  SerializedHashedAccountsKeyBytes,
				StoreKey:  storetypes.NewKVStoreKey(HashedAccountsStoreName),
			},
			HashedStoragesTableCode: {
				DupSorted:   true,
				KeyBytes:    SerializedHashedStoragesKeyBytes,
				SubKeyBytes: SerializedHashedStoragesSubKeyBytes,
				StoreKey:    storetypes.NewKVStoreKey(HashedStoragesStoreName),
			},
		}

		engineAPIURL := appOpts.Get(params.FlagExecutionEngineURL).(string)
		rpcURL := appOpts.Get(params.FlagRPCURL).(string)
		jwtSecretPath := appOpts.Get(params.FlagExecutionEngineJWTSecretPath).(string)
		rpcJWTRefreshInterval, err := time.ParseDuration(appOpts.Get(params.FlagRPCJWTRefreshInterval).(string))
		if err != nil {
			return nil, err
		}

		rpcCheckInterval, err := time.ParseDuration(appOpts.Get(params.FlagRPCCheckInterval).(string))
		if err != nil {
			return nil, err
		}

		strEthChainID := appOpts.Get(params.FlagEthChainID).(string)
		ethChainID := new(big.Int)
		_, ok := ethChainID.SetString(strEthChainID, 0)
		if !ok {
			return nil, fmt.Errorf("failed to convert eth chainID to big.Int")
		}

		evmConfig, err := evmexec.NewEVMConfig(ethChainID, rpcJWTRefreshInterval, rpcCheckInterval, engineAPIURL, rpcURL, jwtSecretPath)
		if err != nil {
			return nil, err
		}

		evmClient, err := evmexec.NewEVMClient(ctx, evmConfig, logger)
		if err != nil {
			return nil, err
		}

		return evmClient, nil

	default:
		return nil, fmt.Errorf("invalid execution client type")
	}
}
