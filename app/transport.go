package app

import (
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

// var (
// just for testing iterator functionality
// 	startKey      = []byte{1, 2, 3, 4}
// 	invalidEndKey = []byte{0, 2, 3, 4}
// 	sks           = []storetypes.StoreKey{}
// )

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

		data := buffer[:n]

		// fmt.Println("BUFFER: ", buffer)

		operation := data[0]
		fmt.Println("OPERATION: ", operation)

		tableCode = uint8(data[1])
		fmt.Println("TABLE CODE: ", tableCode)

		if _, ok := ethExecutionTableInfo[tableCode]; !ok {
			response = append([]byte{STATUS_ERROR}, []byte(ErrStoreNotFound.Error())...)
			app.writeToConn(conn, response)
			continue
		}

		switch operation {
		case OP_GET:

			fmt.Println("GET REQUEST LEN: ", len(data))

			var key []byte

			table := app.executionTablesInfo[tableCode]
			if len(data) != 2+table.KeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			key = data[2 : 2+table.KeyBytes]

			value, err := app.Get(tableCode, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = append([]byte{STATUS_SUCCESS}, value...)

		case OP_PUT:
			var (
				key   []byte
				value []byte
			)

			table := app.executionTablesInfo[tableCode]

			if table.DupSorted {
				if len(data) <= 2+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2 : 2+table.KeyBytes+table.SubKeyBytes]
				value = data[2+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2 : 2+table.KeyBytes]
				value = data[2+table.KeyBytes:]
			}

			if err := app.Put(tableCode, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = append([]byte{STATUS_SUCCESS}, value...)

		case OP_DELETE:
			// for DupSorted Table in delete request if:
			// - key and value both are specified: only that entry is deleted
			// - only key is specified: all entries for that key is deleted

			fmt.Println("DELETE REQUEST LEN: ", len(data))
			var (
				key               []byte
				keyIncludesSubkey bool
			)

			table := app.executionTablesInfo[tableCode]

			if (!table.DupSorted && len(data) != 2+table.KeyBytes) ||
				(table.DupSorted && len(data) != 2+table.KeyBytes && len(data) != 2+table.KeyBytes+table.SubKeyBytes) {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if table.DupSorted && len(data) == 2+table.KeyBytes+table.SubKeyBytes {
				keyIncludesSubkey = true
			}

			key = data[2:]

			if err := app.Delete(tableCode, key, keyIncludesSubkey); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = []byte{STATUS_SUCCESS}

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

func (app *ScalerizeApp) Get(tableCode uint8, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrStoreNotFound
	}

	if table.DupSorted {
		iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(key, storetypes.PrefixEndBytes(key))
		defer iterator.Close()

		if !iterator.Valid() {
			return nil, ErrDataNotFound
		}

		return iterator.Value(), nil
	}

	if !app.CommitMultiStore().GetKVStore(table.StoreKey).Has(key) {
		return nil, ErrDataNotFound
	}

	return app.CommitMultiStore().GetKVStore(table.StoreKey).Get(key), nil
}

func (app *ScalerizeApp) Put(tableCode uint8, key []byte, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrStoreNotFound
	}

	app.executionCacheMultistore.GetKVStore(table.StoreKey).Set(key, value)

	return nil
}

func (app *ScalerizeApp) Delete(tableCode uint8, key []byte, keyIncludesSubkey bool) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrStoreNotFound
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)

	if table.DupSorted {
		if keyIncludesSubkey {
			store.Delete(key)
			return nil
		}

		iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(key, storetypes.PrefixEndBytes(key))
		defer iterator.Close()
		fmt.Println("KEY: ", key)

		for ; iterator.Valid(); iterator.Next() {
			fmt.Println("DELETE KEY: ", iterator.Key())
			store.Delete(iterator.Key())
		}

		return nil
	}

	store.Delete(key)

	return nil
}

func (app *ScalerizeApp) Write() {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	app.executionCacheMultistore.Write()
	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()
}

// first gets the first entry in the table and sets the cursor to that key
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
	if _, err := conn.Write(response); err != nil {
		app.Logger().Error(err.Error())
	}
}
