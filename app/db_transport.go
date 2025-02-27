package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	evmexec "github.com/aerius-labs/scalerize/execution/evm"
)

type handleDatabaseConnection func(conn net.Conn)

func (app *ScalerizeApp) StartDBRouter(clientType string) {
	var hConn handleDatabaseConnection

	os.Remove(socketPath)

	switch clientType {
	case evmexec.EVM:
		hConn = app.ethHandleDatabaseConnection
	default:
		panic(ErrInvalidExecutionClient)
	}

	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		panic(fmt.Errorf("failed to create socket directory: %w", err))
	}

	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	app.Logger().Info("Listening on: ", socketPath)

	for {
		fmt.Println("CONNECTING TO UNIX SOCKET SERVER FOR DB")
		conn, err := l.Accept()
		if err != nil {
			app.Logger().Error("Error accepting connection to Scalerize DB Router: ", err)
			continue
		}

		app.Logger().Info("New client connected to Scalerize Database Router")

		fmt.Println("New client connected to Scalerize Database Router")

		go hConn(conn)
	}
}
