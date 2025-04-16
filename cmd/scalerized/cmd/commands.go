package cmd

import (
	"errors"
	"io"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"cosmossdk.io/log"
	confixcmd "cosmossdk.io/tools/confix/cmd"

	"github.com/aerius-labs/scalerize/app"
	"github.com/aerius-labs/scalerize/app/params"
	sclient "github.com/aerius-labs/scalerize/client"
	"github.com/aerius-labs/scalerize/execution/evm"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
)

func initRootCmd(rootCmd *cobra.Command, txConfig client.TxConfig, basicManager module.BasicManager) {
	cfg := sdk.GetConfig()
	cfg.Seal()

	rootCmd.AddCommand(
		genutilcli.InitCmd(basicManager, app.DefaultNodeHome),
		debug.Cmd(),
		confixcmd.ConfigCommand(),
		pruning.Cmd(newApp, app.DefaultNodeHome),
		snapshot.Cmd(newApp),
	)

	server.AddCommands(rootCmd, app.DefaultNodeHome, newApp, appExport, func(startCmd *cobra.Command) {
		startCmd.PersistentFlags().String(params.FlagExecutionClientType, evm.EVM, "Specifies the execution client type (e.g., EVM, SVM, etc.)")
		startCmd.PersistentFlags().String(params.FlagExecutionEngineURL, evm.DefaultEngineAPIURL, "URL for the Ethereum execution engine API")
		startCmd.PersistentFlags().String(params.FlagRPCURL, evm.DefaultRPCURL, "URL for Ethereum RPC calls")
		startCmd.PersistentFlags().String(params.FlagExecutionEngineJWTSecretPath, evm.DefaultJWTSecretPath, "Path to a JWT secret to use for the authenticated engine-API RPC server")
		startCmd.PersistentFlags().String(params.FlagRPCCheckInterval, evm.DefaultRPCCheckInterval.String(), "Ethereum RPC server startup check interval")
		startCmd.PersistentFlags().String(params.FlagRPCJWTRefreshInterval, evm.DefaultRPCJWTRefreshInterval.String(), "Ethereum execution engine API jwt refresh interval")
		startCmd.PersistentFlags().String(params.FlagEthChainID, evm.DefaultEthChainID, "Ethereum execution client chain id")
		startCmd.PersistentFlags().String(params.FlagSocketPath, evm.DefaultSocketPath, "Unix socket path used for IPC between Scalerize and the execution client")
		startCmd.PersistentFlags().String(params.FlagCometBFTRPCAddress, evm.DefaultCometBFTRPCAddress, "RPC address specified in config.toml")
	})

	// add keybase, auxiliary RPC, query, genesis, and tx child commands
	rootCmd.AddCommand(
		server.StatusCommand(),
		genutilcli.Commands(txConfig, basicManager, app.DefaultNodeHome),
		sclient.NewTestnetCmd(basicManager, banktypes.GenesisBalancesIterator{}),
		queryCommand(),
		txCommand(),
		keys.Commands(),
	)
}

func queryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.ValidatorCommand(),
		server.QueryBlockCmd(),
		authcmd.QueryTxsByEventsCmd(),
		server.QueryBlocksCmd(),
		authcmd.QueryTxCmd(),
		server.QueryBlockResultsCmd(),
	)

	return cmd
}

func txCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
	)

	return cmd
}

// newApp is an appCreator
func newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	baseappOptions := server.DefaultBaseappOptions(appOpts)
	app, err := app.NewScalerizeApp(logger, db, traceStore, true, appOpts, baseappOptions...)
	if err != nil {
		panic(err)
	}

	return app
}

// appExport creates a new app (optionally at a given height) and exports state.
func appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	var (
		scalerizeApp *app.ScalerizeApp
		err          error
	)

	// this check is necessary as we use the flag in x/upgrade.
	// we can exit more gracefully by checking the flag here.
	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home not set")
	}

	viperAppOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("appOpts is not viper.Viper")
	}

	// overwrite the FlagInvCheckPeriod
	viperAppOpts.Set(server.FlagInvCheckPeriod, 1)
	appOpts = viperAppOpts

	if height != -1 {
		scalerizeApp, err = app.NewScalerizeApp(logger, db, traceStore, false, appOpts)
		if err != nil {
			return servertypes.ExportedApp{}, err
		}

		if err := scalerizeApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		scalerizeApp, err = app.NewScalerizeApp(logger, db, traceStore, true, appOpts)
		if err != nil {
			return servertypes.ExportedApp{}, err
		}
	}

	return scalerizeApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}
