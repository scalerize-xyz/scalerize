package app

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"math/big"
	"sync"
	"time"

	scalerizeabci "github.com/aerius-labs/scalerize/abci"
	evmexec "github.com/aerius-labs/scalerize/execution/evm"
	evmtypes "github.com/aerius-labs/scalerize/x/evm/types"

	abci "github.com/cometbft/cometbft/abci/types"
	crypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cometbft/cometbft/rpc/client/http"
	dbm "github.com/cosmos/cosmos-db"

	// cosmossdkclient "github.com/cosmos/cosmos-sdk/client"
	// "github.com/cosmos/cosmos-sdk/codec/types"

	"cosmossdk.io/core/appconfig"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	evmkeeper "github.com/aerius-labs/scalerize/x/evm/keeper"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/mempool"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	_ "cosmossdk.io/api/cosmos/tx/config/v1" // import for side-effects
	"github.com/aerius-labs/scalerize/app/params"
	_ "github.com/cosmos/cosmos-sdk/x/auth"           // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/auth/tx/config" // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/bank"           // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/consensus"      // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/distribution"   // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/mint"           // import for side-effects
	_ "github.com/cosmos/cosmos-sdk/x/staking"        // import for side-effects
)

// DefaultNodeHome default home directories for the application daemon
var DefaultNodeHome string

//go:embed app.yaml
var AppConfigYAML []byte

var (
	_          runtime.AppI            = (*ScalerizeApp)(nil)
	_          servertypes.Application = (*ScalerizeApp)(nil)
	socketPath string
)

// ScalerizeApp extends an ABCI application, but with most of its parameters exported.
// They are exported for convenience in creating helper functions, as object
// capabilities aren't needed for testing.
type ScalerizeApp struct {
	*runtime.App
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	// keepers
	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.Keeper
	StakingKeeper         *stakingkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	ConsensusParamsKeeper consensuskeeper.Keeper
	EVMKeeper             evmkeeper.Keeper

	// simulation manager
	sm *module.SimulationManager

	executionTablesInfo      map[uint8]tableInfo
	executionCacheMultistore storetypes.CacheMultiStore
	rwMutex                  sync.RWMutex
}

func init() {
	var err error
	clienthelpers.EnvPrefix = "SCALERIZE"
	DefaultNodeHome, err = clienthelpers.GetNodeHomeDirectory(".scalerized")
	if err != nil {
		panic(err)
	}
}

// AppConfig returns the default app config.
func AppConfig() depinject.Config {
	return depinject.Configs(
		appconfig.LoadYAML(AppConfigYAML),
		depinject.Supply(
			&appv1alpha1.Config{}, // hack until https://github.com/cosmos/cosmos-sdk/pull/21042
			// supply custom module basics
			map[string]module.AppModuleBasic{
				genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
			},
		),
	)
}

// NewScalerizeApp returns a reference to an initialized ScalerizeApp.
func NewScalerizeApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) (*ScalerizeApp, error) {
	var (
		app                   = &ScalerizeApp{}
		appBuilder            *runtime.AppBuilder
		abciHandler           scalerizeabci.ABCIHandler
		ctx                   = context.Background()
		ensureClientCreatedCh = make(chan bool)
	)

	if err := depinject.Inject(
		depinject.Configs(
			AppConfig(),
			depinject.Supply(
				logger,
				appOpts,
			),
		),
		&appBuilder,
		&app.appCodec,
		&app.legacyAmino,
		&app.txConfig,
		&app.interfaceRegistry,
		&app.AccountKeeper,
		&app.BankKeeper,
		&app.StakingKeeper,
		&app.DistrKeeper,
		&app.ConsensusParamsKeeper,
	); err != nil {
		return nil, err
	}

	socketPath = appOpts.Get(params.FlagSocketPath).(string)
	clientType := appOpts.Get(params.FlagExecutionClientType).(string)

	switch clientType {
	case evmexec.EVM:
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

		go func() {
			if err := evmClient.Start(ctx, ensureClientCreatedCh); err != nil {
				panic(err)
			}
		}()

		if abciHandler, err = evmexec.NewEVMABCIHandler(ctx, evmClient); err != nil {
			app.Logger().Error("failed to create EVM ABCI Handler")
			return nil, err
		}

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

	default:
		return nil, fmt.Errorf("invalid execution client type")
	}

	baseAppOptions = append(baseAppOptions, func(ba *baseapp.BaseApp) {
		ba.SetPrepareProposal(abciHandler.PrepareProposal())
		ba.SetProcessProposal(abciHandler.ProcessProposal())
		ba.SetPreBlocker(abciHandler.PreBlock())
		ba.SetEndBlocker(abciHandler.EndBlock())
		ba.SetMempool(mempool.NoOpMempool{})
		// ba.SetBeginBlocker(abciHandler.BeginBlocker())
		// ba.SetExtendVoteHandler(abciHandler.ExtendVote())
		// ba.SetVerifyVoteExtensionHandler(abciHandler.VerifyVoteExtension())
	})

	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)

	// register streaming services
	if err := app.RegisterStreamingServices(appOpts, app.kvStoreKeys()); err != nil {
		return nil, err
	}

	if err := app.RegisterStores(evmtypes.EVMStoreKey); err != nil {
		return nil, err
	}

	for _, table := range app.executionTablesInfo {
		if err := app.RegisterStores(table.StoreKey); err != nil {
			return nil, err
		}
	}

	/****  Module Options ****/

	// create the simulation manager and define the order of the modules for deterministic simulations
	// NOTE: this is not required apps that don't use the simulator for fuzz testing transactions
	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, make(map[string]module.AppModuleSimulation, 0))
	app.sm.RegisterStoreDecoders()

	if err := app.Load(loadLatest); err != nil {
		return nil, err
	}

	for i := range app.ModuleManager.Modules {
		fmt.Println("MODULE: ", i)
	}

	for _, k := range app.GetStoreKeys() {
		fmt.Println("STORE KEYS: ", k)
	}

	fmt.Printf("Registered Message Router: %+v\n", app.MsgServiceRouter())
	fmt.Printf("Registered GRPC Router: %+v\n", app.GRPCQueryRouter())

	go app.StartDBRouter()
	// go func() {
	// 	for {
	// 		time.Sleep(10 * time.Second)
	// 		fmt.Println("HERE BEFORE WRITE: ", app.CommitMultiStore().WorkingHash())
	// 		fmt.Println("HERE LAST COMMIT APP HASH BEFORE WRITE: ", app.CommitMultiStore().LastCommitID().Hash)
	// 	}
	// }()

	for _, key := range app.GetStoreKeys() {
		fmt.Println("STORE KEYS: ", key.Name())
	}

	<-ensureClientCreatedCh

	return app, nil
}

// LegacyAmino returns ScalerizeApp's amino codec.
func (app *ScalerizeApp) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// GetKey returns the KVStoreKey for the provided store key.
func (app *ScalerizeApp) GetKey(storeKey string) *storetypes.KVStoreKey {
	sk := app.UnsafeFindStoreKey(storeKey)
	kvStoreKey, ok := sk.(*storetypes.KVStoreKey)
	if !ok {
		return nil
	}
	return kvStoreKey
}

func (app *ScalerizeApp) kvStoreKeys() map[string]*storetypes.KVStoreKey {
	keys := make(map[string]*storetypes.KVStoreKey)
	for _, k := range app.GetStoreKeys() {
		if kv, ok := k.(*storetypes.KVStoreKey); ok {
			keys[kv.Name()] = kv
		}
	}

	return keys
}

// SimulationManager implements the SimulationApp interface
func (app *ScalerizeApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *ScalerizeApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	app.App.RegisterAPIRoutes(apiSvr, apiConfig)
	// register swagger API in app.go so that other applications can override easily
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

func CreateCometBFTClient(rpcEndpoint string) (*http.HTTP, error) {
	client, err := http.New(rpcEndpoint, "/websocket")
	if err != nil {
		return nil, err
	}

	// Optionally, you can start the client
	err = client.Start()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func CreateCosmosClient(cometBFTClient *http.HTTP) (client.Context, error) {
	interfaceRegistry := types.NewInterfaceRegistry()
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	// Create the client context
	clientCtx := client.Context{}.
		WithClient(cometBFTClient).
		WithCodec(marshaler).
		WithInterfaceRegistry(interfaceRegistry)

	return clientCtx, nil
}

func GetProof(clientCtx client.Context, storeKey string, key []byte) ([]byte, *crypto.ProofOps, error) {
	height := clientCtx.Height
	// ABCI queries at height less than or equal to 2 are not supported.
	// Base app does not support queries for height less than or equal to 1.
	// Therefore, a query at height 2 would be equivalent to a query at height 3
	if height <= 2 {
		return nil, nil, fmt.Errorf("proof queries at height <= 2 are not supported")
	}

	abciReq := abci.RequestQuery{
		Path:   fmt.Sprintf("store/%s/key", storeKey),
		Data:   key,
		Height: height,
		Prove:  true,
	}

	abciRes, err := clientCtx.QueryABCI(abciReq)
	if err != nil {
		return nil, nil, err
	}

	return abciRes.Value, abciRes.ProofOps, nil
}

func (app *ScalerizeApp) InitializeCommitMultiStore(db dbm.DB, logger log.Logger, metricGatherer metrics.StoreMetrics) error {
	// Initialize the CommitMultiStore
	cms := store.NewCommitMultiStore(db, logger, metricGatherer)

	// Mount necessary stores
	for _, table := range app.executionTablesInfo {
		cms.MountStoreWithDB(table.StoreKey, storetypes.StoreTypeIAVL, db)
	}

	// Load the latest version
	err := cms.LoadLatestVersion()
	if err != nil {
		return err
	}

	// Set the CommitMultiStore
	app.SetCMS(cms)

	return nil
}
