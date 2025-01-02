package app

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	storetypes "cosmossdk.io/store/types"
)

// todo: if we are not able to figure out the number of key bytes for a particular store
// maybe they can vary, then we can send the number of key bytes in the request only
const (
	// DbTx and DbTxMut
	OP_PUT    byte = 1
	OP_GET    byte = 2
	OP_DELETE byte = 3
	OP_WRITE  byte = 4

	// DbCursorRO
	OP_FIRST      byte = 5
	OP_SEEK_EXACT byte = 6
	OP_SEEK       byte = 7
	OP_NEXT       byte = 8
	OP_PREV       byte = 9
	OP_LAST       byte = 10
	OP_CURRENT    byte = 11
)

const (
	STATUS_SUCCESS byte = 1
	STATUS_ERROR   byte = 0
)

var (
	// just for testing iterator functionality
	startKey      = []byte{1, 2, 3, 4}
	invalidEndKey = []byte{0, 2, 3, 4}

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

	// just for testing iterator functionality
	storeKey := lookUpTable[2].StoreKey

	iterator := app.CommitMultiStore().GetCommitKVStore(storeKey).Iterator(nil, nil)

	fmt.Printf("Current key of the Iterator before: %v\n", iterator.Key())

	fmt.Println("Moving Iterator to next key 1st time")
	iterator.Next()
	fmt.Printf("Current key of the Iterator after: %v\n", iterator.Key())

	fmt.Println("Moving Iterator to next key 2nd time")
	iterator.Next()
	fmt.Printf("Current key of the Iterator after: %v\n", iterator.Key())

	iterator.Close()

	iterator = app.CommitMultiStore().GetCommitKVStore(storeKey).Iterator(nil, nil)
	start, end := iterator.Domain()
	fmt.Printf("iterator HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)

	fmt.Println("all keys in the iterator")
	fmt.Println("----------")
	for ; iterator.Valid(); iterator.Next() {
		key := iterator.Key()
		fmt.Printf("Key: %x\n", key)
	}
	fmt.Println("----------")

	filteredIterator := app.CommitMultiStore().GetKVStore(storeKey).Iterator(startKey, nil)
	start, end = filteredIterator.Domain()
	fmt.Printf("filteredIterator HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)
	fmt.Println("all keys in the filtered iterator")
	fmt.Println("----------")
	for ; filteredIterator.Valid(); filteredIterator.Next() {
		key := filteredIterator.Key()
		fmt.Printf("Key: %x\n", key)
	}
	fmt.Println("----------")

	filteredReverseIterator1 := app.CommitMultiStore().GetKVStore(storeKey).ReverseIterator(startKey, nil)
	start, end = filteredReverseIterator1.Domain()
	fmt.Printf("filteredReverseIterator1 HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)

	fmt.Println("all keys in the filtered reverse iterator 1")
	fmt.Println("----------")
	for ; filteredReverseIterator1.Valid(); filteredReverseIterator1.Next() {
		key := filteredReverseIterator1.Key()
		fmt.Printf("Key: %x\n", key)
	}

	filteredReverseIterator2 := app.CommitMultiStore().GetKVStore(storeKey).ReverseIterator(startKey, invalidEndKey)
	start, end = filteredReverseIterator2.Domain()
	fmt.Printf("filteredReverseIterator2 HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)

	fmt.Println("all keys in the filtered reverse iterator 2")
	fmt.Println("----------")
	for ; filteredReverseIterator2.Valid(); filteredReverseIterator2.Next() {
		key := filteredReverseIterator2.Key()
		fmt.Printf("Key: %x\n", key)
	}

	startNotExistsIterator := app.CommitMultiStore().GetKVStore(storeKey).Iterator([]byte{1, 0, 3, 4}, nil)
	start, end = startNotExistsIterator.Domain()
	fmt.Printf("startNotExistsIterator HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)
	fmt.Printf("startNotExistsIterator current key: %v\n", startNotExistsIterator.Key())
	fmt.Printf("startNotExistsIterator is valid: %t\n", startNotExistsIterator.Valid())

	if lookUpTable[2].IteratorsKey == nil {
		fmt.Println("Current store iterator key is nil")
	} else {
		fmt.Println("Current store iterator key is not nil")
		fmt.Println("Here it is: ", lookUpTable[2].IteratorsKey)
	}

	lastEntryIsOnlyEntryIterator := app.CommitMultiStore().GetKVStore(storeKey).Iterator(nil, []byte{0, 2, 3, 4})
	start, end = lastEntryIsOnlyEntryIterator.Domain()
	fmt.Printf("lastEntryIsOnlyEntryIterator HeaderNumbers Iterator Domain: start: %v, end %v\n", start, end)
	fmt.Printf("lastEntryIsOnlyEntryIterator current key: %v\n", lastEntryIsOnlyEntryIterator.Key())
	fmt.Printf("lastEntryIsOnlyEntryIterator is valid: %t\n", lastEntryIsOnlyEntryIterator.Valid())
}

// first gets the first entry in the table and sets the cursor to that key
func (app *ScalerizeApp) First(storeNumber uint8) ([]byte, []byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, nil, err
	}

	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil, ErrStoreIsEmpty
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	return iterator.Key(), iterator.Value(), nil
}

// seek exact (sets the key to cursor to the exact key and return the key value pair)
// or (just sets the iterator to the next greater one)
func (app *ScalerizeApp) SeekExact(storeNumber uint8, key []byte) ([]byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, err
	}

	// if key does not exists then the iterator start domain is set to the next greater key
	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrExactOrGreaterKeyNotExists
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	if !bytes.Equal(key, iterator.Key()) {
		return nil, ErrKeyNotExists
	}

	return iterator.Value(), nil
}

// seek (sets the key to cursor to the (exact or next greater key) and return the key value pair)
func (app *ScalerizeApp) Seek(storeNumber uint8, key []byte) ([]byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, err
	}

	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrExactOrGreaterKeyNotExists
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	return iterator.Value(), nil
}

// next returns the next from the current entry in the table, but if
// current key is not set of the cursor then first entry is returned
func (app *ScalerizeApp) Next(storeNumber uint8) ([]byte, []byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, nil, err
	}

	if store.IteratorsKey == nil {
		return app.First(storeNumber)
	}

	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(store.IteratorsKey, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil, ErrCurrentIteratorKeyIsInvalid
	}

	iterator.Next()
	if !iterator.Valid() {
		return nil, nil, ErrCannotIteratePrevOrNextWhenCurrentKeyIsOnlyEntry
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	return iterator.Key(), iterator.Value(), nil
}

// prev returns the previous from the current entry of the table but if
// current key is not set then the last entry is returned
func (app *ScalerizeApp) Prev(storeNumber uint8) ([]byte, []byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, nil, err
	}

	if store.IteratorsKey == nil {
		return app.Last(storeNumber)
	}

	iterator := app.CommitMultiStore().GetCommitKVStore(store.StoreKey).Iterator(nil, store.IteratorsKey)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil, ErrCannotIteratePrevOrNextWhenCurrentKeyIsOnlyEntry
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	return iterator.Key(), iterator.Value(), nil
}

// last gets the last entry in the table and sets the cursor to that key
func (app *ScalerizeApp) Last(storeNumber uint8) ([]byte, []byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, nil, err
	}

	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).ReverseIterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil, ErrStoreIsEmpty
	}

	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

	return iterator.Key(), iterator.Value(), nil
}

func (app *ScalerizeApp) Current(storeNumber uint8) ([]byte, []byte, error) {
	store, err := getTable(storeNumber)
	if err != nil {
		return nil, nil, err
	}

	if store.IteratorsKey == nil {
		return nil, nil, ErrCurrentKeyIsNotSet
	}

	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(store.IteratorsKey, storetypes.PrefixEndBytes(store.IteratorsKey))
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil, ErrKeyNotExists
	}

	return iterator.Key(), iterator.Value(), nil
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
