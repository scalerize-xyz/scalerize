package app

import (
	"bytes"
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
	// DbTx and DbTxMut for both regular and dup-sorted tables
	OP_PUT    byte = 1
	OP_GET    byte = 2
	OP_DELETE byte = 3
	OP_WRITE  byte = 4

	// DbCursorRO for both regular and dup-sorted tables
	OP_FIRST      byte = 5
	OP_SEEK_EXACT byte = 6
	OP_SEEK       byte = 7
	OP_NEXT       byte = 8
	OP_PREV       byte = 9
	OP_LAST       byte = 10
	OP_CURRENT    byte = 11

	// DbCursorRW for both regular and dup-sorted tables
	OP_UPSERT         byte = 12
	OP_INSERT         byte = 13
	OP_APPEND         byte = 14
	OP_DELETE_CURRENT byte = 15

	// DbDupCursorRO for dup-sorted tables
	OP_NEXT_DUP           byte = 16
	OP_NEXT_NO_DUP        byte = 17
	OP_NEXT_DUP_VAL       byte = 18
	OP_SEEK_BY_KEY_SUBKEY byte = 19

	OP_DROP_CURSOR byte = 16
)

const (
	STATUS_SUCCESS byte = 1
	STATUS_ERROR   byte = 0
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

		data := buffer[:n]

		// fmt.Println("BUFFER: ", buffer)

		operation := data[0]
		fmt.Println("OPERATION: ", operation)

		tableCode = uint8(data[1])
		fmt.Println("TABLE CODE: ", tableCode)

		if _, ok := app.executionTablesInfo[tableCode]; !ok {
			response = append([]byte{STATUS_ERROR}, []byte(ErrTableNotFound.Error())...)
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

			response = []byte{STATUS_SUCCESS}

		case OP_DELETE:
			// for DupSorted Table in delete request if:
			// - key and subkey both are specified: only that entry is deleted
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

		case OP_FIRST:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.First(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_SEEK_EXACT:
			table := app.executionTablesInfo[tableCode]

			if len(data) != 2+CursorIDBytes+table.KeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			key := data[2+CursorIDBytes:]
			value, err := app.SeekExact(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, value...)
			}

		case OP_SEEK:
			table := app.executionTablesInfo[tableCode]

			if len(data) != 2+CursorIDBytes+table.KeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			key := data[2+CursorIDBytes:]
			resp, err := app.Seek(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_NEXT:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.Next(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_PREV:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.Prev(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_LAST:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.Last(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_CURRENT:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.Current(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_UPSERT:
			var (
				key   []byte
				value []byte
			)

			table := app.executionTablesInfo[tableCode]

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes:]
			}

			if err := app.Upsert(tableCode, cursorId, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = []byte{STATUS_SUCCESS}

		case OP_INSERT:
			var (
				key   []byte
				value []byte
			)

			table := app.executionTablesInfo[tableCode]

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes:]
			}

			if err := app.Insert(tableCode, cursorId, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = []byte{STATUS_SUCCESS}

		case OP_APPEND:
			var (
				key   []byte
				value []byte
			)

			table := app.executionTablesInfo[tableCode]

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+CursorIDBytes : 2+CursorIDBytes+table.KeyBytes]
				value = data[2+CursorIDBytes+table.KeyBytes:]
			}

			if err := app.Append(tableCode, cursorId, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = []byte{STATUS_SUCCESS}
		case OP_DELETE_CURRENT:
			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			if err := app.DeleteCurrent(tableCode, cursorId); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
			}

		case OP_NEXT_DUP:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.NextDup(false, tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_NEXT_NO_DUP:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.NextNoDup(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_NEXT_DUP_VAL:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) != 2+CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])
			resp, err := app.NextDup(true, tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_SEEK_BY_KEY_SUBKEY:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) != 2+CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+CursorIDBytes])

			key := data[2+CursorIDBytes:]
			resp, err := app.SeekByKeySubkey(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}
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
		return nil, ErrTableNotFound
	}

	if table.DupSorted {
		iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(key, storetypes.PrefixEndBytes(key))
		defer iterator.Close()

		if !iterator.Valid() {
			return nil, ErrDataNotFound
		}

		subkey := iterator.Key()[table.KeyBytes:]
		value := iterator.Value()

		response := append(subkey, value...)
		return response, nil
	}

	if !app.CommitMultiStore().GetKVStore(table.StoreKey).Has(key) {
		return nil, ErrDataNotFound
	}

	return app.CommitMultiStore().GetKVStore(table.StoreKey).Get(key), nil
}

func (app *ScalerizeApp) Put(tableCode uint8, key, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrTableNotFound
	}

	app.executionCacheMultistore.GetKVStore(table.StoreKey).Set(key, value)

	return nil
}

func (app *ScalerizeApp) Delete(tableCode uint8, key []byte, keyIncludesSubkey bool) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrTableNotFound
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
// same for dup-sorted tables
func (app *ScalerizeApp) First(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	fmt.Println("ITERATOR POSITION BEFORE FIRST: ", ethIteratorsCurrentKey[cursorID])

	iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrTableIsEmpty
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER FIRST: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)
	return response, nil
}

// seek exact (sets the key to cursor to the exact key and return the key value pair)
// or (just sets the iterator to the next greater one)
// for dup-sorted tables it returns the value at the key and the smallest subkey lexicographically
func (app *ScalerizeApp) SeekExact(tableCode uint8, cursorID [8]byte, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE SEEK_EXACT: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	// if key does not exists then the iterator start domain is set to the next greater key
	iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrExactOrGreaterKeyNotExists
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER SEEK_EXACT: ", ethIteratorsCurrentKey[cursorID])

	if (table.DupSorted && !bytes.HasPrefix(iterator.Key(), key)) ||
		(!table.DupSorted && !bytes.Equal(key, iterator.Key())) {
		return nil, nil
	}

	return iterator.Value(), nil
}

// seek (sets the key to cursor to the (exact or next greater key) and return the key value pair)
// for dup-sorted tables it returns the value at the key and the smallest subkey lexicographically
// and if key not exists it does the same for next greater key if exists
// no need to add different logic for dup-sorted tables
func (app *ScalerizeApp) Seek(tableCode uint8, cursorID [8]byte, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE SEEK: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrExactOrGreaterKeyNotExists
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER SEEK: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// next returns the next from the current entry in the table, but if
// current key is not set of the cursor then first entry is returned
// works the same for dup-sorted tables
func (app *ScalerizeApp) Next(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE NEXT: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		return app.First(tableCode, cursorID)
	}

	iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).Iterator(currentKey, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrCurrentIteratorKeyIsInvalid
	}

	iterator.Next()
	if !iterator.Valid() {
		return nil, ErrCannotIterateToNextFromLast
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER NEXT: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// prev returns the previous from the current entry of the table but if
// current key is not set then the last entry is returned
// works the same for dup-sorted tables
func (app *ScalerizeApp) Prev(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE PREV: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		return app.Last(tableCode, cursorID)
	}

	iterator := app.CommitMultiStore().GetCommitKVStore(table.StoreKey).ReverseIterator(nil, currentKey)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrCannotIterateToPrevFromFirst
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER PREV: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// last gets the last entry in the table and sets the cursor to that key
// works the same for dup-sorted tables
func (app *ScalerizeApp) Last(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE LAST: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	iterator := app.CommitMultiStore().GetKVStore(table.StoreKey).ReverseIterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrTableIsEmpty
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()

	fmt.Println("ITERATOR POSITION AFTER LAST: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

func (app *ScalerizeApp) Current(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE CURRENT: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		return nil, ErrCurrentKeyIsNotSet
	}

	store := app.CommitMultiStore().GetKVStore(table.StoreKey)
	if !store.Has(currentKey) {
		return nil, ErrKeyNotExists
	}

	value := app.CommitMultiStore().GetKVStore(table.StoreKey).Get(currentKey)

	fmt.Println("ITERATOR POSITION AFTER CURRENT: ", ethIteratorsCurrentKey[cursorID])

	response := append(currentKey, value...)

	return response, nil
}

// upsert is same as put but also set the cursor key
func (app *ScalerizeApp) Upsert(tableCode uint8, cursorID [8]byte, key, value []byte) error {
	fmt.Println("ITERATOR POSITION BEFORE UPSERT: ", ethIteratorsCurrentKey[cursorID])

	fmt.Println("UPSERT KEY: ", key)
	if err := app.Put(tableCode, key, value); err != nil {
		return err
	}

	ethIteratorsCurrentKey[cursorID] = key

	fmt.Println("ITERATOR POSITION BEFORE UPSERT: ", ethIteratorsCurrentKey[cursorID])

	return nil
}

// insert will insert a row at a given key. If the key is already
// present, the operation will result in an error. And also set the cursor key
// in case of dup-sorted tables also, if an entry exists for a KEY(not KEY+SUBKEY) it fails
func (app *ScalerizeApp) Insert(tableCode uint8, cursorID [8]byte, key, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	fmt.Println("ITERATOR POSITION BEFORE INSERT: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrTableNotFound
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)
	if table.DupSorted {
		k := key[:table.KeyBytes]
		iterator := store.Iterator(k, storetypes.PrefixEndBytes(k))
		defer iterator.Close()

		if iterator.Valid() {
			fmt.Println("THIS CASE 1")
			return ErrKeyAlreadyPresent
		}
	} else {
		if store.Has(key) {
			fmt.Println("THIS CASE 2")
			return ErrKeyAlreadyPresent
		}
	}

	store.Set(key, value)

	ethIteratorsCurrentKey[cursorID] = key

	fmt.Println("ITERATOR POSITION BEFORE INSERT: ", ethIteratorsCurrentKey[cursorID])

	return nil
}

// append stores new entries in the table, but:
// the key (only key not KEY+SUBKEY) should be
// lexicographically equal or more than the greatest key present in the table
// in regular table if key is same as the greatest key then the value is updated
func (app *ScalerizeApp) Append(tableCode uint8, cursorID [8]byte, k, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	fmt.Println("ITERATOR POSITION BEFORE APPEND: ", ethIteratorsCurrentKey[cursorID])

	var key []byte

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrTableNotFound
	}

	if table.DupSorted {
		key = k[:table.KeyBytes]
	} else {
		key = k
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)
	iterator := store.ReverseIterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		store.Set(k, value)
		return nil
	}

	greatestKeyPresent := iterator.Key()[:table.KeyBytes]
	if bytes.Compare(key, greatestKeyPresent) < 0 {
		return ErrCannotAppendIfKeyIsLessThanCurrrentGreatestKey
	}

	store.Set(k, value)
	ethIteratorsCurrentKey[cursorID] = k

	fmt.Println("ITERATOR POSITION BEFORE APPEND: ", ethIteratorsCurrentKey[cursorID])

	return nil
}

// delete the current key for the cursor. If current key is not set than fails
// after deleting moves to next key
// unset the cursor after deleting the current if current key is the last one
func (app *ScalerizeApp) DeleteCurrent(tableCode uint8, cursorID [8]byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	fmt.Println("ITERATOR POSITION BEFORE DELETE CURRENT: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return ErrTableNotFound
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		return ErrCurrentKeyIsNotSet
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)
	iterator := store.Iterator(currentKey, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return ErrCurrentIteratorKeyIsInvalid
	}

	store.Delete(currentKey)

	iterator.Next()
	if !iterator.Valid() {
		delete(ethIteratorsCurrentKey, cursorID)
	} else {
		ethIteratorsCurrentKey[cursorID] = iterator.Key()
	}

	fmt.Println("ITERATOR POSITION BEFORE DELETE CURRENT: ", ethIteratorsCurrentKey[cursorID])

	return nil
}

// next_dup returns the next entry with same key (not key+subkey)
// if next entry is not with the same key then it return None
func (app *ScalerizeApp) NextDup(onlyVal bool, tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	var response []byte

	fmt.Println("ITERATOR POSITION BEFORE NEXT DUP: ", ethIteratorsCurrentKey[cursorID])
	fmt.Println("ONLY VAL: ", onlyVal)

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		resp, err := app.First(tableCode, cursorID)
		if onlyVal && err == nil {
			resp = resp[table.KeyBytes:]
		}

		return resp, err
	}

	key := currentKey[:table.KeyBytes]
	fmt.Println("CURRENT KEY: ", currentKey)
	fmt.Println("KEY: ", key)

	iterator := app.CommitMultiStore().GetCommitKVStore(table.StoreKey).Iterator(currentKey, storetypes.PrefixEndBytes(key))
	defer iterator.Close()

	if !iterator.Valid() {
		fmt.Println("ERROR 1")
		return nil, ErrCurrentIteratorKeyIsInvalid
	}

	iterator.Next()

	if !iterator.Valid() {
		fmt.Println("ERROR 2")
		return nil, nil
	}

	if bytes.HasPrefix(iterator.Key(), key) {
		ethIteratorsCurrentKey[cursorID] = iterator.Key()

		fmt.Println("ITERATOR POSITION AFTER NEXT DUP: ", ethIteratorsCurrentKey[cursorID])

		if onlyVal {
			response = append(iterator.Key()[table.KeyBytes:], iterator.Value()...)
		} else {
			response = append(iterator.Key(), iterator.Value()...)
		}
	}

	return response, nil
}

// next_no_dup returns the first entry for the next key(not key+subkey)
// if current key is greatest then return nil
func (app *ScalerizeApp) NextNoDup(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE NEXT NO DUP: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	currentKey, ok := ethIteratorsCurrentKey[cursorID]
	if !ok {
		return app.First(tableCode, cursorID)
	}

	key := currentKey[:table.KeyBytes]
	fmt.Println("CURRENT KEY: ", currentKey)
	fmt.Println("KEY: ", key)

	iterator := app.CommitMultiStore().GetCommitKVStore(table.StoreKey).Iterator(storetypes.PrefixEndBytes(key), nil)
	defer iterator.Close()

	if !iterator.Valid() {
		fmt.Println("THIS ERROR 1")
		return nil, nil
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()
	fmt.Println("ITERATOR POSITION AFTER NEXT NO DUP: ", ethIteratorsCurrentKey[cursorID])
	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// seek_by_key_subkey returns only value
// positions the cursor at the entry greater than or equal to the provided key/subkey pair
// if key(not key+subkey) does not exists, then returns nil
// if key exists but subkey is greater than the greatest subkey for that key, then returns nil
// if key and subkey exists, it returns value at that entry
// if key exists and subkey does not exists, then it returns the next greater key/subkey pair for that key
func (app *ScalerizeApp) SeekByKeySubkey(tableCode uint8, cursorID [8]byte, k []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	fmt.Println("ITERATOR POSITION BEFORE SEEK BY KEY SUBKEY: ", ethIteratorsCurrentKey[cursorID])

	table, ok := app.executionTablesInfo[tableCode]
	if !ok {
		return nil, ErrTableNotFound
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	key := k[:table.KeyBytes]
	iterator := app.CommitMultiStore().GetCommitKVStore(table.StoreKey).Iterator(k, storetypes.PrefixEndBytes(key))
	defer iterator.Close()

	if !iterator.Valid() {
		fmt.Println("KEY DOES NOT EXISTS")
		return nil, nil
	}

	ethIteratorsCurrentKey[cursorID] = iterator.Key()
	fmt.Println("ITERATOR POSITION AFTER SEEK BY KEY SUBKEY: ", ethIteratorsCurrentKey[cursorID])

	response := append(iterator.Key()[table.KeyBytes:], iterator.Value()...)
	return response, nil
}

func (app *ScalerizeApp) writeToConn(conn net.Conn, response []byte) {
	if _, err := conn.Write(response); err != nil {
		app.Logger().Error(err.Error())
	}
}
