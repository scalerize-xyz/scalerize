package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"

	storetypes "cosmossdk.io/store/types"
)

// todo: if we are not able to figure out the number of key bytes for a particular store
// maybe they can vary, then we can send the number of key bytes in the request only

// todo: if multiple cursor instances are being created in reth, then we can add a new field
// in the reth implementation which stores the curr key for the cursor and send the curr key
// with each request.
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

	// DbCursorRW
	OP_UPSERT         byte = 12
	OP_INSERT         byte = 13
	OP_APPEND         byte = 14
	OP_DELETE_CURRENT byte = 15
)

const (
	STATUS_SUCCESS byte = 1
	STATUS_ERROR   byte = 0
)

var (
	// just for testing iterator functionality
	startKey      = []byte{1, 2, 3, 4}
	invalidEndKey = []byte{0, 2, 3, 4}
)

func (app *ScalerizeApp) StartDBRouter() {
	os.Remove(socketPath)

	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()

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
			response  []byte
			tableCode uint8
		)

		// 1st byte contains the operation
		// 2nd byte contains the table code
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

		// fmt.Println("BUFFER: ", buffer)

		operation := buffer[0]
		fmt.Println("OPERATION: ", operation)

		tableCode = uint8(buffer[1])
		fmt.Println("TABLE CODE: ", tableCode)

		switch operation {
		case OP_GET:
			switch tableCode {
			case HashedAccountsTableCode:
				if len(buffer) < 2+HashedAccountsKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[2 : 2+HashedAccountsKeyBytes])
				value, err := app.Get(false, storeName, HashedAccountDataKey[:])
				if err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = append([]byte{STATUS_SUCCESS}, value...)

			// when get is called for DupSortedTables, the first key-value pair in the sorted table is returned
			case HashedStoragesTableCode:
				if len(buffer) < 2+HashedStoragesKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[2 : 2+HashedStoragesKeyBytes])
				value, err := app.Get(true, storeName, nil)
				if err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = append([]byte{STATUS_SUCCESS}, value...)

			default:
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidTableCode.Error())...)
			}

		case OP_PUT:
			// for PUT request 3rd and 4th bytes are used to know bytes taken by value
			value_len := binary.BigEndian.Uint16(buffer[2:4])
			fmt.Println("VALUE LEN: ", value_len)

			switch tableCode {
			case HashedAccountsTableCode:

				if len(buffer) < 4+HashedAccountsKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[4 : 4+HashedAccountsKeyBytes])
				if _, ok := app.executionDBStoreKeys[storeName]; !ok {
					if response = app.createAndAddStoreKey(storeName); response != nil {
						break
					}
				}

				value := buffer[4+HashedAccountsKeyBytes : 4+HashedAccountsKeyBytes+value_len]
				if err := app.Put(storeName, HashedAccountDataKey[:], value); err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = append([]byte{STATUS_SUCCESS}, []byte(value)...)

			case HashedStoragesTableCode:
				if len(buffer) < 4+HashedStoragesKeyBytes+HashedStoragesSubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[4 : 4+HashedStoragesKeyBytes])
				if _, ok := app.executionDBStoreKeys[string(storeName)]; !ok {
					response = append([]byte{STATUS_ERROR}, []byte(ErrStoreNotFound.Error())...)
					break
				}

				subkey := buffer[4+HashedStoragesKeyBytes : 4+HashedStoragesKeyBytes+HashedStoragesSubKeyBytes]
				value := buffer[4+HashedStoragesKeyBytes+HashedStoragesSubKeyBytes : 4+HashedStoragesKeyBytes+HashedStoragesSubKeyBytes+value_len]
				if err := app.Put(storeName, subkey, value); err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = append([]byte{STATUS_SUCCESS}, []byte(value)...)

			default:
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidTableCode.Error())...)
			}

		case OP_DELETE:
			// for DELETE request 3rd and 4th bytes are used to know bytes taken by value. If 0 it means value is not specified
			// subkey is not needed in the request
			// for HashedAccountsTable in delete request the whole substore needs to be deleted for the account address (for now I am deleting all entries)
			// for HashedStorages in delete request if:
			// - key and value both are specified: only that entry is deleted
			// - only key is specified: all entries except entry for account data is remained
			value_len := binary.BigEndian.Uint16(buffer[2:4])
			fmt.Println("VALUE LEN: ", value_len)

			switch tableCode {
			case HashedAccountsTableCode:
				if len(buffer) < 4+HashedAccountsKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[4 : 4+HashedAccountsKeyBytes])
				if _, ok := app.executionDBStoreKeys[storeName]; !ok {
					response = append([]byte{STATUS_ERROR}, []byte(ErrStoreNotFound.Error())...)
					break
				}

				if err := app.Delete(false, storeName, nil); err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = []byte{STATUS_SUCCESS}

			case HashedStoragesTableCode:
				if len(buffer) < 4+HashedStoragesKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				storeName := string(buffer[4 : 4+HashedStoragesKeyBytes])
				if _, ok := app.executionDBStoreKeys[storeName]; !ok {
					if response = app.createAndAddStoreKey(storeName); response != nil {
						break
					}
				}

				var value []byte
				if value_len == 0 {
					value = nil
				} else {
					value = buffer[4+HashedStoragesKeyBytes : 4+HashedStoragesKeyBytes+value_len]
				}

				if err := app.Delete(true, storeName, value); err != nil {
					response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
					break
				}

				response = []byte{STATUS_SUCCESS}

			default:
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidTableCode.Error())...)
			}
			// key := buffer[2 : 2+lookUpTable[storeNumber].NoOfKeyBytes]
			// fmt.Println("KEY: ", key)

			// err := app.Delete(storeNumber, key)

			// if err != nil {
			// 	response = []byte{STATUS_ERROR}
			// 	response = append(response, []byte(err.Error())...)
			// } else {
			// 	response = []byte{STATUS_SUCCESS}
			// }

		case OP_WRITE:
			app.Write()
			response = []byte{STATUS_SUCCESS}

		// case OP_FIRST:
		// 	key, value, err := app.First(storeNumber)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, append(key, value...)...)
		// 	}

		// case OP_SEEK_EXACT:
		// 	key := buffer[2 : 2+lookUpTable[storeNumber].NoOfKeyBytes]
		// 	fmt.Println("KEY: ", key)
		// 	value, err := app.SeekExact(storeNumber, key)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, value...)
		// 	}

		// case OP_SEEK:
		// 	key := buffer[2 : 2+lookUpTable[storeNumber].NoOfKeyBytes]
		// 	fmt.Println("KEY: ", key)
		// 	value, err := app.Seek(storeNumber, key)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, value...)
		// 	}

		// case OP_NEXT:
		// 	key, value, err := app.Next(storeNumber)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, append(key, value...)...)
		// 	}

		// case OP_PREV:
		// 	key, value, err := app.Prev(storeNumber)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, append(key, value...)...)
		// 	}

		// case OP_LAST:
		// 	key, value, err := app.Last(storeNumber)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, append(key, value...)...)
		// 	}

		// case OP_CURRENT:
		// 	key, value, err := app.Current(storeNumber)
		// 	if err != nil {
		// 		response = []byte{STATUS_ERROR}
		// 		response = append(response, []byte(err.Error())...)
		// 	} else {
		// 		response = []byte{STATUS_SUCCESS}
		// 		response = append(response, append(key, value...)...)
		// 	}

		default:
			response = []byte{STATUS_ERROR}
			response = append(response, []byte(ErrInvalidOperationCode.Error())...)
		}

		app.writeToConn(conn, response)
	}
}

func (app *ScalerizeApp) Get(dupSortedTable bool, storeName string, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	storeKey, ok := app.executionDBStoreKeys[storeName]
	if !ok {
		return nil, ErrStoreNotFound
	}

	// when get is called for DupSortedTables, the first key-value pair in the sorted table is returned
	// key
	if dupSortedTable {
		iterator := app.CommitMultiStore().GetKVStore(storeKey).Iterator(nil, nil)
		defer iterator.Close()

		if !iterator.Valid() {
			return nil, ErrTableIsEmpty
		}

		k := iterator.Key()
		if bytes.Equal(k, HashedAccountDataKey[:]) {
			return nil, ErrTableIsEmpty
		}

		value := iterator.Value()
		if len(value) == 0 {
			return nil, ErrDataNotFound
		}

		// for ; iterator.Valid(); iterator.Next() {
		// 	key := iterator.Key()
		// 	value := iterator.Value()
		// 	fmt.Printf("Key: %x, Value: %x\n", key, value)
		// }

		return value, nil
	}

	value := app.CommitMultiStore().GetKVStore(storeKey).Get(key)
	if len(value) == 0 {
		return nil, ErrDataNotFound
	}

	return value, nil
}

func (app *ScalerizeApp) Put(storeName string, key []byte, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	storeKey, ok := app.executionDBStoreKeys[storeName]
	if !ok {
		return ErrStoreNotFound
	}

	app.executionCacheMultistore.GetKVStore(storeKey).Set(key, value)

	return nil
}

func (app *ScalerizeApp) Delete(dupSortedTable bool, storeName string, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	storeKey, ok := app.executionDBStoreKeys[storeName]
	if !ok {
		return ErrStoreNotFound
	}

	store := app.executionCacheMultistore.GetKVStore(storeKey)
	iterator := store.Iterator(nil, nil)
	defer iterator.Close()

	if dupSortedTable {
		if value == nil {
			for ; iterator.Valid() && !bytes.Equal(iterator.Key(), HashedAccountDataKey[:]); iterator.Next() {
				k := iterator.Key()
				store.Delete(k)
			}

			return nil
		}

		for ; iterator.Valid(); iterator.Next() {
			if bytes.Equal(iterator.Value(), value) {
				store.Delete(iterator.Key())
				return nil
			}
		}
	}

	for ; iterator.Valid(); iterator.Next() {
		k := iterator.Key()
		store.Delete(k)
	}

	return nil
}

func (app *ScalerizeApp) Write() {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	app.executionCacheMultistore.Write()
	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()
}

// // first gets the first entry in the table and sets the cursor to that key
// func (app *ScalerizeApp) First(storeName string) ([]byte, []byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE FIRST: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(nil, nil)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, nil, ErrStoreIsEmpty
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER FIRST: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Key(), iterator.Value(), nil
// }

// // seek exact (sets the key to cursor to the exact key and return the key value pair)
// // or (just sets the iterator to the next greater one)
// func (app *ScalerizeApp) SeekExact(storeNumber uint8, key []byte) ([]byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE SEEK_EXACT: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// if key does not exists then the iterator start domain is set to the next greater key
// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(key, nil)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, ErrExactOrGreaterKeyNotExists
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER SEEK_EXACT: ", lookUpTable[storeNumber].IteratorsKey)

// 	if !bytes.Equal(key, iterator.Key()) {
// 		return nil, ErrKeyNotExists
// 	}

// 	return iterator.Value(), nil
// }

// // seek (sets the key to cursor to the (exact or next greater key) and return the key value pair)
// func (app *ScalerizeApp) Seek(storeNumber uint8, key []byte) ([]byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE SEEK: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, err
// 	}

// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(key, nil)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, ErrExactOrGreaterKeyNotExists
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER SEEK: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Value(), nil
// }

// // next returns the next from the current entry in the table, but if
// // current key is not set of the cursor then first entry is returned
// func (app *ScalerizeApp) Next(storeNumber uint8) ([]byte, []byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE NEXT: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	if store.IteratorsKey == nil {
// 		return app.First(storeNumber)
// 	}

// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(store.IteratorsKey, nil)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, nil, ErrCurrentIteratorKeyIsInvalid
// 	}

// 	iterator.Next()
// 	if !iterator.Valid() {
// 		return nil, nil, ErrCannotIterateToNextFromLast
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER NEXT: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Key(), iterator.Value(), nil
// }

// // prev returns the previous from the current entry of the table but if
// // current key is not set then the last entry is returned
// func (app *ScalerizeApp) Prev(storeNumber uint8) ([]byte, []byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE PREV: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	if store.IteratorsKey == nil {
// 		return app.Last(storeNumber)
// 	}

// 	iterator := app.CommitMultiStore().GetCommitKVStore(store.StoreKey).ReverseIterator(nil, store.IteratorsKey)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, nil, ErrCannotIterateToPrevFromFirst
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER PREV: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Key(), iterator.Value(), nil
// }

// // last gets the last entry in the table and sets the cursor to that key
// func (app *ScalerizeApp) Last(storeNumber uint8) ([]byte, []byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE LAST: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).ReverseIterator(nil, nil)
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, nil, ErrStoreIsEmpty
// 	}

// 	lookUpTable[storeNumber].IteratorsKey = iterator.Key()

// 	fmt.Println("ITERATOR POSITION AFTER LAST: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Key(), iterator.Value(), nil
// }

// func (app *ScalerizeApp) Insert(storeNumber uint8, key []byte, value []byte) error {
// 	app.rwMutex.Lock()
// 	defer app.rwMutex.Unlock()
// 	// store, err := getTable(storeNumber)
// 	// if err != nil {
// 	// 	return err
// 	// }

// 	// app.CommitMultiStore().GetCommitKVStore()

// 	return nil
// }

// func (app *ScalerizeApp) Current(storeNumber uint8) ([]byte, []byte, error) {
// 	app.rwMutex.RLock()
// 	defer app.rwMutex.RUnlock()

// 	fmt.Println("ITERATOR POSITION BEFORE CURRENT: ", lookUpTable[storeNumber].IteratorsKey)

// 	store, err := getTable(storeNumber)
// 	if err != nil {
// 		return nil, nil, err
// 	}

// 	if store.IteratorsKey == nil {
// 		return nil, nil, ErrCurrentKeyIsNotSet
// 	}

// 	iterator := app.CommitMultiStore().GetKVStore(store.StoreKey).Iterator(store.IteratorsKey, storetypes.PrefixEndBytes(store.IteratorsKey))
// 	defer iterator.Close()

// 	if !iterator.Valid() {
// 		return nil, nil, ErrKeyNotExists
// 	}

// 	fmt.Println("ITERATOR POSITION AFTER CURRENT: ", lookUpTable[storeNumber].IteratorsKey)

// 	return iterator.Key(), iterator.Value(), nil
// }

func (app *ScalerizeApp) writeToConn(conn net.Conn, response []byte) {
	// fmt.Println("SENDING RESPONSE:", response)
	if _, err := conn.Write(response); err != nil {
		app.Logger().Error(err.Error())
	}
}

// func getTable(storeNumber uint8) (*TableInfo, error) {
// 	if storeNumber >= NumberOfTables {
// 		return nil, ErrStoreNotFound
// 	}

// 	return &lookUpTable[storeNumber], nil
// }

// func getStoreKey(tableCode uint8)
func (app *ScalerizeApp) createAndAddStoreKey(storeName string) (errResponse []byte) {
	storeKey := storetypes.NewKVStoreKey(storeName)

	if err := app.RegisterStores(storeKey); err != nil {
		errResponse = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
		return
	}

	storeUpgrades := storetypes.StoreUpgrades{
		Added: []string{storeKey.Name()},
	}

	if err := app.CommitMultiStore().LoadVersionAndUpgrade(app.CommitMultiStore().LatestVersion(), &storeUpgrades); err != nil {
		errResponse = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
		return
	}

	app.executionDBStoreKeys[storeName] = storeKey
	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()

	return nil
}
