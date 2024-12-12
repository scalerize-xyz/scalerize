package app

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/aerius-labs/scalerize/abci"
	evmexec "github.com/aerius-labs/scalerize/execution/evm"
	evmtypes "github.com/aerius-labs/scalerize/x/evm/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/labstack/echo/v4"

	"cosmossdk.io/core/appconfig"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	"github.com/aerius-labs/scalerize/x/evm"
	evmkeeper "github.com/aerius-labs/scalerize/x/evm/keeper"
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
	sdk "github.com/cosmos/cosmos-sdk/types"
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
	_ runtime.AppI            = (*ScalerizeApp)(nil)
	_ servertypes.Application = (*ScalerizeApp)(nil)
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
		app         = &ScalerizeApp{}
		appBuilder  *runtime.AppBuilder
		abciHandler abci.ABCIHandler
		ctx         = context.Background()
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

	fmt.Printf("TX CONFIG: %+v\n", app.txConfig)

	evmstorekey := storetypes.NewKVStoreKey(evmtypes.StoreKey)
	evmKeeper := evmkeeper.NewKeeper(evmstorekey, app.appCodec)

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
		ensureClientCreatedCh := make(chan bool)

		go func() {
			if err := evmClient.Start(ctx, ensureClientCreatedCh); err != nil {
				panic(err)
			}
		}()

		go func() {
			e := echo.New()
			ctx := sdk.UnwrapSDKContext(ctx)

			e.GET("/getparams", func(c echo.Context) error {
				params := evmKeeper.GetParams(ctx)
				fmt.Printf("GETPARAMS RESPONSE: %+v\n", params)
				return c.JSON(http.StatusOK, params)
			})

			e.PUT("/setparams", func(c echo.Context) error {
				if err := evmKeeper.SetParams(ctx, evmtypes.Params{
					Name: "scalerize",
				}); err != nil {
					return nil
				}

				params := evmKeeper.GetParams(ctx)
				fmt.Printf("IN SETPARAMS GETPARAMS RESPONSE: %+v\n", params)

				return nil
			})

			e.Start(":3000")
		}()

		<-ensureClientCreatedCh

		if abciHandler, err = evmexec.NewEVMABCIHandler(ctx, evmClient); err != nil {
			app.Logger().Error("failed to create EVM ABCI Handler")
			return nil, err
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

	storeUpgrades := storetypes.StoreUpgrades{
		Added: []string{evmtypes.StoreKey},
	}

	if err := app.CommitMultiStore().LoadVersionAndUpgrade(app.CommitMultiStore().LatestVersion(), &storeUpgrades); err != nil {
		return nil, err
	}

	// evmstorekey := storetypes.NewKVStoreKey(evmtypes.StoreKey)
	if err := app.RegisterStores(evmstorekey); err != nil {
		return nil, err
	}

	// evmKeeper := evmkeeper.NewKeeper(evmstorekey, app.appCodec)
	app.EVMKeeper = *evmKeeper
	if err := app.RegisterModules(evm.NewAppModule(evmKeeper)); err != nil {
		return nil, err
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

	app.MsgServiceRouter()

	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("UPDATING PARAMS")
		resp, err := app.EVMKeeper.UpdateParams(ctx, &evmtypes.MsgUpdateParams{
			Params: evmtypes.Params{},
		})
		if err != nil {
			fmt.Println("ERROR: ", err)
			panic(err)
		}

		fmt.Printf("RESPONSE: %+v\n", resp)

		app.BaseApp.NewContext(false)
	}()

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
