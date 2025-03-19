package app

import (
	"fmt"
	"net"
	"os"

	evmexec "github.com/aerius-labs/scalerize/execution/evm"
)

func (app *ScalerizeApp) StartStateRouter(clientType string) {
	var hConn func(conn net.Conn)

	os.Remove(stateSocketPath)

	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()

	switch clientType {
	case evmexec.EVM:
		hConn = app.ethHandleStateConnection
	default:
		panic(ErrInvalidExecutionClient)
	}

	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: stateSocketPath, Net: "unix"})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	app.Logger().Info("Listening on: ", stateSocketPath)

	for {
		fmt.Println("CONNECTING TO UNIX SOCKET SERVER FOR STATE QUERIES")
		conn, err := l.Accept()
		if err != nil {
			app.Logger().Error("Error accepting connection to Scalerize State Router: ", err)
			continue
		}

		app.Logger().Info("New client connected to Scalerize State Router")

		fmt.Println("New client connected to Scalerize State Router")

		go hConn(conn)
	}
}
