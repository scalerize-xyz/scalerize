package app

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"sync"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/core/appconfig"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
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
	_                  runtime.AppI            = (*ScalerizeApp)(nil)
	_                  servertypes.Application = (*ScalerizeApp)(nil)
	dbSocketPath       string
	stateSocketPath    string
	cometBFTRPCAddress string
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

	dbSocketPath = appOpts.Get(params.FlagDBSocketPath).(string)
	stateSocketPath = appOpts.Get(params.FlagStateSocketPath).(string)
	cometBFTRPCAddress = appOpts.Get(params.FlagCometBFTRPCAddress).(string)

	executionClient, err := app.NewClient(ctx, appOpts, logger)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := executionClient.Start(ctx, ensureClientCreatedCh); err != nil {
			panic(err)
		}
	}()

	baseAppOptions = append(baseAppOptions, func(ba *baseapp.BaseApp) {
		ba.SetPrepareProposal(executionClient.PrepareProposal())
		ba.SetProcessProposal(executionClient.ProcessProposal())
		ba.SetPreBlocker(executionClient.PreBlock())
		ba.SetEndBlocker(executionClient.EndBlock())
		ba.SetMempool(mempool.NoOpMempool{})
	})

	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)

	// register streaming services
	if err := app.RegisterStreamingServices(appOpts, app.kvStoreKeys()); err != nil {
		return nil, err
	}

	for _, table := range app.executionTablesInfo {
		if err := app.RegisterStores(table.StoreKey); err != nil {
			return nil, err
		}
	}

	/****  Module Options ****/

	fmt.Println("CHAIN ID: ", app.ChainID())
	// create the simulation manager and define the order of the modules for deterministic simulations
	// NOTE: this is not required apps that don't use the simulator for fuzz testing transactions
	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, make(map[string]module.AppModuleSimulation, 0))
	app.sm.RegisterStoreDecoders()

	if err := app.Load(loadLatest); err != nil {
		return nil, err
	}

	clientType := appOpts.Get(params.FlagExecutionClientType).(string)

	executionClient.SetApp(app.BaseApp)

	go app.StartDBRouter(clientType)
	go app.StartStateRouter(clientType)
	go executionClient.SetCosmosRPCClient(cometBFTRPCAddress)
	// go func() {
	// 	time.Sleep(10 * time.Second)
	// 	if err := app.StartSyncMonitor(1 * time.Second); err != nil {
	// 		panic(err)
	// 	}
	// }()
	// go func() {
	// 	time.Sleep(20 * time.Second)
	// 	c := app.CommitMultiStore().CacheMultiStore()
	// 	c.GetKVStore(app.executionTablesInfo[0].StoreKey).Set([]byte{3}, []byte{3})
	// 	fmt.Println("GET 2 from cachemultistore", c.GetKVStore(app.executionTablesInfo[0].StoreKey).Get([]byte{3}))
	// 	c.Write()
	// 	fmt.Println("GET 2 from commitmultistore", app.CommitMultiStore().CacheMultiStore().GetKVStore(app.executionTablesInfo[0].StoreKey).Get([]byte{3}))
	// 	// fmt.Println("----------------------------", app.CommitMultiStore().GetKVStore(app.executionTablesInfo[0].StoreKey).Get([]byte{
	// 	// 	32, 0, 0, 0, 0, 0, 0, 0, 254, 212, 140, 188, 17, 169, 63, 100,
	// 	// 	69, 192, 211, 41, 51, 101, 89, 199, 60, 113, 120, 2, 184, 8, 7, 223,
	// 	// 	184, 19, 202, 193, 34, 143, 25, 137,
	// 	// }))

	// 	// store := app.CommitMultiStore().GetKVStore(app.executionTablesInfo[0].StoreKey)
	// 	// iterator := store.Iterator(nil, nil) // This will iterate over all keys
	// 	// defer iterator.Close()

	// 	// fmt.Println("All data in store:", app.executionTablesInfo[0].StoreKey.Name())
	// 	// for ; iterator.Valid(); iterator.Next() {
	// 	// 	key := iterator.Key()
	// 	// 	value := iterator.Value()
	// 	// 	fmt.Printf("Key: %x, Value: %x\n", key, value)
	// 	// 	// For more readable output if your data is UTF-8 strings:
	// 	// 	// fmt.Printf("Key: %s, Value: %s\n", string(key), string(value))
	// 	// }

	// }()

	<-ensureClientCreatedCh

	// for _, storekey := range app.GetStoreKeys() {
	// 	fmt.Printf("STORE KEY: %+v\n", storekey)
	// }

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

// func (app *ScalerizeApp) FinalizeBlock(req *types.RequestFinalizeBlock) (*types.ResponseFinalizeBlock, error) {
// 	fmt.Println("THIS FINALIZE BLOCK")
// 	fmt.Println("WORKING HASH BEFORE FINALIZE BLOCK: ", app.CommitMultiStore().WorkingHash())
// 	resp, err := app.BaseApp.FinalizeBlock(req)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// syncing, err := app.executionClient.SyncingStatus()
// 	// if err != nil {
// 	// 	return nil, err
// 	// }

// 	// fmt.Println("SYNCING IN FINALIZE BLOCK: ", syncing)
// 	fmt.Println("WORKING HASH AFTER FINALIZE BLOCK: ", app.CommitMultiStore().WorkingHash())

// 	return resp, nil
// }

// func (app *ScalerizeApp) StartSyncMonitor(checkInterval time.Duration) error {
// 	cometbftClient, err := createCometBFTClient(cometBFTRPCAddress)
// 	if err != nil {
// 		return err
// 	}

// 	cosmosSDKClient, err := createCosmosClient(cometbftClient)
// 	if err != nil {
// 		return err
// 	}

// 	for {
// 		fmt.Println("*******************")
// 		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		status, err := cosmosSDKClient.Client.Status(ctx)
// 		fmt.Println("SYNCING STATUS: ", status.SyncInfo.CatchingUp)
// 		cancel()

// 		if err == nil {
// 			fmt.Println("SYNCING STATUS: ", status.SyncInfo.CatchingUp)
// 			app.Logger().Info("Updated sync status",
// 				"catching_up", status.SyncInfo.CatchingUp,
// 				"latest_block_height", status.SyncInfo.LatestBlockHeight)
// 		} else {
// 			app.Logger().Error("Failed to get node status", "error", err)
// 		}

// 		time.Sleep(checkInterval)
// 	}
// }

// func GetProof(clientCtx client.Context, storeKey string, key []byte) ([]byte, *crypto.ProofOps, error) {
// 	height := clientCtx.Height
// 	fmt.Println("HEIGHT: ", height)
// 	// ABCI queries at height less than or equal to 2 are not supported.
// 	// Base app does not support queries for height less than or equal to 1.
// 	// Therefore, a query at height 2 would be equivalent to a query at height 3
// 	if height <= 2 {
// 		return nil, nil, fmt.Errorf("proof queries at height <= 2 are not supported")
// 	}

// 	abciReq := abci.RequestQuery{
// 		Path:   fmt.Sprintf("store/%s/key", storeKey),
// 		Data:   key,
// 		Height: height,
// 		Prove:  true,
// 	}

// 	abciRes, err := clientCtx.QueryABCI(abciReq)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	return abciRes.Value, abciRes.ProofOps, nil
// }

// func (app *ScalerizeApp) InitializeCommitMultiStore(db dbm.DB, logger log.Logger, metricGatherer metrics.StoreMetrics) error {
// 	// Initialize the CommitMultiStore
// 	cms := store.NewCommitMultiStore(db, logger, metricGatherer)

// 	// Mount necessary stores
// 	for _, table := range app.executionTablesInfo {
// 		cms.MountStoreWithDB(table.StoreKey, storetypes.StoreTypeIAVL, db)
// 	}

// 	// Load the latest version
// 	err := cms.LoadLatestVersion()
// 	if err != nil {
// 		return err
// 	}

// 	// Set the CommitMultiStore
// 	app.SetCMS(cms)

// 	return nil
// }
