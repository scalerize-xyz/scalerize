package app

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	storetypes "cosmossdk.io/store/types"
)

const (
	OP_PUT    byte = 1
	OP_GET    byte = 2
	OP_DELETE byte = 3
	OP_WRITE  byte = 4
)

const (
	STATUS_SUCCESS byte = 1
	STATUS_ERROR   byte = 0
)

var (
	cacheMultistore storetypes.CacheMultiStore
	rwMutex         sync.RWMutex
)

func (app *ScalerizeApp) StartDBRouter() {
	os.Remove(socketPath)

	cacheMultistore = app.CommitMultiStore().CacheMultiStore()

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

		go app.handleConnection(conn)
	}
}

func (app *ScalerizeApp) handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("STARTING HANDLING CONNECTION")

	for {
		var (
			response    []byte
			storeNumber uint8
		)

		// 1st byte contains the operation
		// 2nd byte contains the store number
		buffer := make([]byte, 4096)

		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Client closed connection")
			} else {
				app.Logger().Error("Connection error: " + err.Error())
			}
			return
		}

		if n == 0 {
			continue
		}

		fmt.Println("BUFFER: ", buffer)

		operation := buffer[0]
		fmt.Println("OPERATION: ", operation)

		storeNumber = uint8(buffer[1])
		fmt.Println("STORE NUMBER: ", storeNumber)

		key := buffer[2 : 2+lookUpTable[storeNumber].NoOfKeyBytes]
		fmt.Println("KEY: ", key)

		switch operation {
		case OP_GET:
			rwMutex.RLock()
			value, err := app.Get(storeNumber, key)
			rwMutex.RUnlock()

			if err != nil {
				response = []byte{STATUS_ERROR}
				response = append(response, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
				response = append(response, value...)
			}

		case OP_PUT:
			value := buffer[2+lookUpTable[storeNumber].NoOfKeyBytes:]
			fmt.Println("VALUE: ", value)

			rwMutex.Lock()
			err := app.Put(storeNumber, key, value)
			rwMutex.Unlock()

			if err != nil {
				response = []byte{STATUS_ERROR}
				response = append(response, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
				response = append(response, []byte(value)...)
			}

		case OP_DELETE:
			rwMutex.Lock()
			err := app.Delete(storeNumber, key)
			rwMutex.Unlock()

			if err != nil {
				response = []byte{STATUS_ERROR}
				response = append(response, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
			}

		case OP_WRITE:
			rwMutex.Lock()
			app.Write()
			rwMutex.Unlock()

			response = []byte{STATUS_SUCCESS}
		default:
			response = []byte{STATUS_ERROR}
			response = append(response, []byte(ErrInvalidOperationCode.Error())...)
		}

		app.writeToConn(conn, response)
	}
}

func (app *ScalerizeApp) Get(storeNumber uint8, key []byte) ([]byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, err
	}

	kvstore := app.CommitMultiStore().GetKVStore(store.StoreKey)
	value := kvstore.Get(key)
	if len(value) == 0 {
		return nil, ErrDataNotFound
	}

	return value, nil
}

func (app *ScalerizeApp) Put(storeNumber uint8, key []byte, value []byte) error {
	store, err := getTable(storeNumber)
	if err != nil {
		return err
	}

	fmt.Println("HEADERNUMBERS storekey: ", store.StoreKey)

	cacheMultistore.GetKVStore(store.StoreKey).Set(key, value)

	return nil
}

func (app *ScalerizeApp) Delete(storeNumber uint8, key []byte) error {
	store, err := getTable(storeNumber)
	if err != nil {
		return err
	}

	cacheMultistore.GetKVStore(store.StoreKey).Delete(key)

	return nil
}

func (app *ScalerizeApp) Write() {
	cacheMultistore.Write()
	cacheMultistore = app.CommitMultiStore().CacheMultiStore()
}

func (app *ScalerizeApp) writeToConn(conn net.Conn, response []byte) {
	fmt.Println("SENDING RESPONSE:", response)
	if _, err := conn.Write(response); err != nil {
		app.Logger().Error(err.Error())
	}
}

func getTable(storeNumber uint8) (*TableInfo, error) {
	if storeNumber >= NumberOfTables {
		return nil, ErrStoreNotFound
	}

	return &lookUpTable[storeNumber], nil
}
