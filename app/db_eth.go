package app

import (
	"bytes"
	"io"
	"net"

	storetypes "cosmossdk.io/store/types"
	"github.com/aerius-labs/scalerize/execution/evm"
)

func (app *ScalerizeApp) ethHandleDatabaseConnection(conn net.Conn) {
	defer conn.Close()

	// fmt.Println("STARTING HANDLING CONNECTION")

	for {
		// app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()

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
				// fmt.Println("Client closed connection")
			} else {
				// app.Logger().Error("Connection error: " + err.Error())
			}
			return
		}

		// app.CommitMultiStore().GetKVStore(app.executionTablesInfo[0].StoreKey).Set([]byte{1}, []byte{1})

		// rpcEndpoint := "http://localhost:26657" // Replace with your node's RPC endpoint
		// cometBFTClient, err := CreateCometBFTClient(rpcEndpoint)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// defer cometBFTClient.Stop() // Remember to stop the client when you're done

		// // The block height you're interested in
		// cosmosClient, err := CreateCosmosClient(cometBFTClient)
		// if err != nil {
		// 	log.Fatal(err)
		// }

		// res, err := cosmosClient.Client.ABCIQueryWithOptions(context.Background(), "/store/hashed_storages/key", []byte{1}, client.ABCIQueryOptions{
		// 	Prove: true,
		// })
		// if err != nil {
		// 	log.Fatal(err)
		// }

		// fmt.Printf("PROOF: %+v\n", res)

		if n == 0 {
			continue
		}

		data := buffer[:n]

		operation := data[0]
		// fmt.Println("OPERATION: ", operation)

		if operation != OP_WRITE {
			tableCode = uint8(data[1])
			// fmt.Println("TABLE CODE: ", tableCode)

			// if tableCode == 0 {
			// 	fmt.Println("OPERATION: ", operation)
			// }
			if _, err := app.getTable(tableCode); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				app.writeToConn(conn, response)
				continue
			}
		}

		switch operation {
		case OP_GET:
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

			// fmt.Println("DELETE REQUEST LEN: ", len(data))
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
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			resp, err := app.First(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_SEEK_EXACT:
			table := app.executionTablesInfo[tableCode]

			if len(data) != 2+evm.CursorIDBytes+table.KeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			key := data[2+evm.CursorIDBytes:]
			value, err := app.SeekExact(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				// fmt.Println("SEEK EXACT RESPONSE: ", response)
			} else {
				response = append([]byte{STATUS_SUCCESS}, value...)
				// fmt.Println("SEEK EXACT RESPONSE: ", response)
			}

		case OP_SEEK:
			table := app.executionTablesInfo[tableCode]

			if len(data) != 2+evm.CursorIDBytes+table.KeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			key := data[2+evm.CursorIDBytes:]
			resp, err := app.Seek(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
				// fmt.Println("SEEK RESPONSE: ", response)
			}

		case OP_NEXT:
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			resp, err := app.Next(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_PREV:
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			resp, err := app.Prev(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_LAST:
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			resp, err := app.Last(tableCode, cursorId)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_CURRENT:
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
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
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]

				// fmt.Printf("KEY STORAGE: %v\n", key)
				// fmt.Printf("VALUE STORAGE: %v\n", value)
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes:]

				// fmt.Printf("KEY ACCOUNT: %v\n", key)
				// fmt.Printf("VALUE ACCOUNT: %v\n", value)
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
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes:]
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
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			if table.DupSorted {
				if len(data) <= 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]
			} else {
				if len(data) <= 2+table.KeyBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				key = data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes]
				value = data[2+evm.CursorIDBytes+table.KeyBytes:]
			}

			if err := app.Append(tableCode, cursorId, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
				break
			}

			response = []byte{STATUS_SUCCESS}
		case OP_DELETE_CURRENT:
			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
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

			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
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

			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			resp, err := app.NextNoDup(tableCode, cursorId)
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

			if len(data) != 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])

			key := data[2+evm.CursorIDBytes:]
			resp, err := app.SeekByKeySubkey(tableCode, cursorId, key)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_DELETE_CURRENT_DUPLICATES:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) != 2+evm.CursorIDBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			if err := app.DeleteCurrentDuplicates(tableCode, cursorId); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
			}

		case OP_APPEND_DUP:
			table := app.executionTablesInfo[tableCode]
			if !table.DupSorted {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if len(data) <= 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			key := data[2+evm.CursorIDBytes : 2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes]
			value := data[2+evm.CursorIDBytes+table.KeyBytes+table.SubKeyBytes:]

			var cursorId [8]byte
			copy(cursorId[:], data[2:2+evm.CursorIDBytes])
			if err := app.AppendDup(tableCode, cursorId, key, value); err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = []byte{STATUS_SUCCESS}
			}

		default:
			response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidOperationCode.Error())...)
		}

		app.writeToConn(conn, response)
	}
}

// Get: returns the value at a particular key
// for dup-sorted tables, it returns the first(lexicographically) entry for the key specified
func (app *ScalerizeApp) Get(tableCode uint8, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	if table.DupSorted {
		iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(key, storetypes.PrefixEndBytes(key))
		defer iterator.Close()

		if !iterator.Valid() {
			return nil, nil
		}

		subkey := iterator.Key()[table.KeyBytes:]
		value := iterator.Value()

		response := append(subkey, value...)
		return response, nil
	}

	if !app.executionCacheMultistore.GetKVStore(table.StoreKey).Has(key) {
		return nil, nil
	}

	return app.executionCacheMultistore.GetKVStore(table.StoreKey).Get(key), nil
}

// Put: adds a new entry
func (app *ScalerizeApp) Put(tableCode uint8, key, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	app.executionCacheMultistore.GetKVStore(table.StoreKey).Set(key, value)
	// fmt.Println("GET IN PUT: ", app.executionCacheMultistore.GetKVStore(table.StoreKey).Get(key))

	return nil
}

// Delete: deletes entry for the key specified
// for dup-sorted tables, deletes entry at the key and subkey
// but if subkey is not specified, then deletes all entries for the key specified
func (app *ScalerizeApp) Delete(tableCode uint8, key []byte, keyIncludesSubkey bool) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)

	if table.DupSorted {
		if keyIncludesSubkey {
			store.Delete(key)
			return nil
		}

		iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(key, storetypes.PrefixEndBytes(key))
		defer iterator.Close()
		// fmt.Println("KEY: ", key)

		for ; iterator.Valid(); iterator.Next() {
			// fmt.Println("DELETE KEY: ", iterator.Key())
			store.Delete(iterator.Key())
		}

		return nil
	}

	store.Delete(key)

	return nil
}

func (app *ScalerizeApp) Write() {
	// fmt.Println("CURSOR LENGTH: ", len(evm.EthIteratorsCurrentKey))
	// fmt.Println("WRITE CALLED")
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("WORKING HASH BEFORE: ", app.CommitMultiStore().WorkingHash())
	// fmt.Println("BEFORE WRITE: ", app.CommitMultiStore().WorkingHash())
	// fmt.Println("LAST COMMIT APP HASH BEFORE WRITE: ", app.CommitMultiStore().LastCommitID().Hash)
	// fmt.Println("CURRENT COMMIT APP HASH BEFORE WRITE: ", app.CommitMultiStore().Commit().Hash)
	app.executionCacheMultistore.Write()

	// fmt.Println("WORKING HASH AFTER: ", app.CommitMultiStore().WorkingHash())

	// accountbytes := make(map[[40]byte][]byte)
	// accounts := [][]byte{}
	// store := app.executionCacheMultistore.GetKVStore(app.executionTablesInfo[0].StoreKey)
	// iterator := store.Iterator(nil, nil) // This will iterate over all keys
	// defer iterator.Close()

	// fmt.Println("All data in store:", app.executionTablesInfo[0].StoreKey.Name())
	// for ; iterator.Valid(); iterator.Next() {
	// 	// key := iterator.Key()
	// 	// value := iterator.Value()
	// 	// var fixedKey [40]byte
	// 	// copy(fixedKey[:], key)
	// 	// accountbytes[fixedKey] = value
	// 	accounts = append(accounts, iterator.Key())
	// 	// fmt.Printf("Key: %x, Value: %x\n", key, value)
	// 	// accounts++
	// 	// For more readable output if your data is UTF-8 strings:
	// 	// fmt.Printf("Key: %s, Value: %s\n", string(key), string(value))
	// }

	// fmt.Println("ACCOUNTS: ", len(accounts))
	// fmt.Println(accounts)
	// var storageSlots int
	// store = app.executionCacheMultistore.GetKVStore(app.executionTablesInfo[1].StoreKey)
	// iterator = store.Iterator(nil, nil) // This will iterate over all keys
	// defer iterator.Close()

	// fmt.Println("All data in store:", app.executionTablesInfo[1].StoreKey.Name())
	// for ; iterator.Valid(); iterator.Next() {
	// 	// key := iterator.Key()
	// 	// value := iterator.Value()
	// 	// fmt.Printf("Key: %x, Value: %x\n", key, value)
	// 	storageSlots++
	// 	// For more readable output if your data is UTF-8 strings:
	// 	// fmt.Printf("Key: %s, Value: %s\n", string(key), string(value))
	// }

	// fmt.Println("STORAGES: ", storageSlots)
	// fmt.Println("AFTER WRITE", app.CommitMultiStore().WorkingHash())
	// fmt.Println("LAST COMMIT APP HASH AFTER WRITE: ", app.CommitMultiStore().LastCommitID().Hash)

	app.executionCacheMultistore = app.CommitMultiStore().CacheMultiStore()

	// fmt.Println("CURSOR LEN:", len(evm.EthIteratorsCurrentKey))
	// evm.EthIteratorsCurrentKeyLock.Lock()
	// defer evm.EthIteratorsCurrentKeyLock.Unlock()
	// delete(evm.EthIteratorsCurrentKey, cursorID)
	// fmt.Println("CURSOR LEN AFTER: ", len(evm.EthIteratorsCurrentKey))
}

// First: returns the first entry in the table and sets the cursor to that key
// same for dup-sorted tables
func (app *ScalerizeApp) First(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	// fmt.Println("ITERATOR POSITION BEFORE FIRST: ", evm.EthIteratorsCurrentKey[cursorID])

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER FIRST: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)
	return response, nil
}

// SeekExact: sets the key to cursor to the exact key and return the key value pair
// if key does not exists then just sets the iterator to the next greater one
// for dup-sorted tables it returns the value at the key and the smallest subkey lexicographically
func (app *ScalerizeApp) SeekExact(tableCode uint8, cursorID [8]byte, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE SEEK_EXACT: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	// if key does not exists then the iterator start domain is set to the next greater key
	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER SEEK_EXACT: ", evm.EthIteratorsCurrentKey[cursorID])

	if (table.DupSorted && !bytes.HasPrefix(iterator.Key(), key)) ||
		(!table.DupSorted && !bytes.Equal(key, iterator.Key())) {
		return nil, nil
	}

	response := append(iterator.Key(), iterator.Value()...)
	return response, nil
}

// Seek: (sets the key to cursor to the (exact or next greater key) and return the key value pair)
// for dup-sorted tables it returns the value at the key and the smallest subkey lexicographically
// and if key not exists it does the same for next greater key if exists
// no need to add different logic for dup-sorted tables
func (app *ScalerizeApp) Seek(tableCode uint8, cursorID [8]byte, key []byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE SEEK: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER SEEK: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// Next: returns the next from the current entry in the table, but if
// current key is not set of the cursor then first entry is returned
// works the same for dup-sorted tables
func (app *ScalerizeApp) Next(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE NEXT: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		return app.First(tableCode, cursorID)
	}

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(currentKey, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	iterator.Next()
	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER NEXT: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// Prev: returns the previous from the current entry of the table but if
// current key is not set then the last entry is returned
// works the same for dup-sorted tables
func (app *ScalerizeApp) Prev(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE PREV: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		return app.Last(tableCode, cursorID)
	}

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).ReverseIterator(nil, currentKey)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER PREV: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// Last: returns the last entry in the table and sets the cursor to that key
// works the same for dup-sorted tables
func (app *ScalerizeApp) Last(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE LAST: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).ReverseIterator(nil, nil)
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, nil
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

	// fmt.Println("ITERATOR POSITION AFTER LAST: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// Current: returns the current entry for the cursor
func (app *ScalerizeApp) Current(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("CURSOR ID:", cursorID)
	// fmt.Println("ITERATOR POSITION BEFORE CURRENT: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	evm.EthIteratorsCurrentKeyLock.RLock()
	defer evm.EthIteratorsCurrentKeyLock.RUnlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		return nil, ErrCurrentKeyIsNotSet
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)
	if !store.Has(currentKey) {
		return nil, nil
	}

	value := app.executionCacheMultistore.GetKVStore(table.StoreKey).Get(currentKey)

	// fmt.Println("ITERATOR POSITION AFTER CURRENT: ", evm.EthIteratorsCurrentKey[cursorID])

	response := append(currentKey, value...)

	return response, nil
}

// Upsert: same as put but also set the cursor key
func (app *ScalerizeApp) Upsert(tableCode uint8, cursorID [8]byte, key, value []byte) error {
	// fmt.Println("ITERATOR POSITION BEFORE UPSERT: ", evm.EthIteratorsCurrentKey[cursorID])

	// fmt.Println("UPSERT KEY: ", key)
	// fmt.Println("UPSERT VALUE: ", value)
	if err := app.Put(tableCode, key, value); err != nil {
		return err
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = key

	// fmt.Println("ITERATOR POSITION BEFORE UPSERT: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}

// Insert: inserts a row at a given key. If the key is already
// present, the operation will result in an error. And also set the cursor key
// in case of dup-sorted tables also, if an entry exists for a KEY(not KEY+SUBKEY) it fails
func (app *ScalerizeApp) Insert(tableCode uint8, cursorID [8]byte, key, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("ITERATOR POSITION BEFORE INSERT: ", evm.EthIteratorsCurrentKey[cursorID])
	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)
	if table.DupSorted {
		k := key[:table.KeyBytes]
		iterator := store.Iterator(k, storetypes.PrefixEndBytes(k))
		defer iterator.Close()

		if iterator.Valid() {
			// fmt.Println("THIS CASE 1")
			return ErrKeyAlreadyPresent
		}
	} else {
		if store.Has(key) {
			// fmt.Println("THIS CASE 2")
			return ErrKeyAlreadyPresent
		}
	}

	store.Set(key, value)

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = key

	// fmt.Println("ITERATOR POSITION BEFORE INSERT: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}

// Append: stores new entries in the table, but:
// the key (only key not KEY+SUBKEY) should be
// lexicographically equal or more than the greatest key present in the table
// in regular table if key is same as the greatest key then the value is updated
func (app *ScalerizeApp) Append(tableCode uint8, cursorID [8]byte, k, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("ITERATOR POSITION BEFORE APPEND: ", evm.EthIteratorsCurrentKey[cursorID])

	var key []byte

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
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
		return ErrCannotAppendIfKeyIsLessThanCurrentGreatestKey
	}

	store.Set(k, value)
	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	evm.EthIteratorsCurrentKey[cursorID] = k

	// fmt.Println("ITERATOR POSITION AFTER APPEND: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}

// DeleteCurrent: deletes the current key for the cursor. If current key is not set than fails
// after deleting moves to next key
// unset the cursor after deleting the current if current key is the last one
func (app *ScalerizeApp) DeleteCurrent(tableCode uint8, cursorID [8]byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("ITERATOR POSITION BEFORE DELETE CURRENT: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
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
	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	if !iterator.Valid() {
		delete(evm.EthIteratorsCurrentKey, cursorID)
	} else {
		evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()
	}

	// fmt.Println("ITERATOR POSITION AFTER DELETE CURRENT: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}

// NextDup: returns the next entry with same key (not key+subkey)
// if next entry is not with the same key then it return None
func (app *ScalerizeApp) NextDup(onlyVal bool, tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	var response []byte

	// fmt.Println("ITERATOR POSITION BEFORE NEXT DUP: ", evm.EthIteratorsCurrentKey[cursorID])
	// fmt.Println("ONLY VAL: ", onlyVal)

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		resp, err := app.First(tableCode, cursorID)
		if onlyVal && err == nil {
			resp = resp[table.KeyBytes:]
		}

		return resp, err
	}

	key := currentKey[:table.KeyBytes]
	// fmt.Println("CURRENT KEY: ", currentKey)
	// fmt.Println("KEY: ", key)

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(currentKey, storetypes.PrefixEndBytes(key))
	defer iterator.Close()

	if !iterator.Valid() {
		return nil, ErrCurrentIteratorKeyIsInvalid
	}

	iterator.Next()

	if !iterator.Valid() {
		return nil, nil
	}

	if bytes.HasPrefix(iterator.Key(), key) {
		evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()

		// fmt.Println("ITERATOR POSITION AFTER NEXT DUP: ", evm.EthIteratorsCurrentKey[cursorID])

		if onlyVal {
			response = append(iterator.Key()[table.KeyBytes:], iterator.Value()...)
		} else {
			response = append(iterator.Key(), iterator.Value()...)
		}
	}

	return response, nil
}

// NextNoDup: returns the first entry for the next key(not key+subkey)
// if current key is greatest then return nil
func (app *ScalerizeApp) NextNoDup(tableCode uint8, cursorID [8]byte) ([]byte, error) {
	app.rwMutex.RLock()
	defer app.rwMutex.RUnlock()

	// fmt.Println("ITERATOR POSITION BEFORE NEXT NO DUP: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		return app.First(tableCode, cursorID)
	}

	key := currentKey[:table.KeyBytes]
	// fmt.Println("CURRENT KEY: ", currentKey)
	// fmt.Println("KEY: ", key)

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(storetypes.PrefixEndBytes(key), nil)
	defer iterator.Close()

	if !iterator.Valid() {
		// fmt.Println("THIS ERROR 1")
		return nil, nil
	}

	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()
	// fmt.Println("ITERATOR POSITION AFTER NEXT NO DUP: ", evm.EthIteratorsCurrentKey[cursorID])
	response := append(iterator.Key(), iterator.Value()...)

	return response, nil
}

// SeekByKeySubkey: returns value for a key/subkey
// if key and subkey exists, it returns value at that entry
// if key(not key+subkey) not exists, returns nil but sets the cursor to next entry in the table if exists
// if key exists but subkey does not exists, their are 2 cases:
// 1. if subkey is greater than the greatest subkey for that key, then returns nil but sets the cursor to next entry in the table if exists
// 2. if not, returns the next entry for the key/subkey lexicographically
func (app *ScalerizeApp) SeekByKeySubkey(tableCode uint8, cursorID [8]byte, key []byte) ([]byte, error) {
	// rpcEndpoint := "http://localhost:26657" // Replace with your node's RPC endpoint
	// cometBFTClient, err := createCometBFTClient(rpcEndpoint)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer cometBFTClient.Stop() // Remember to stop the client when you're done

	// Now you can use the client to interact with your CometBFT node
	// For example, to get the latest block:
	// block, err := cometBFTClient.Block(context.Background(), nil)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// cometBFTClient.BlockByHash()

	// fmt.Println("BLOCK CALL COMPLETED")

	// if block.Block != nil && block.Block.Height > 2 {
	// 	fmt.Printf("BLOCK: %+v\n", block)
	// 	fmt.Printf("Latest block height: %d\n", block.Block.Height)
	// 	k := []byte("hashed_storages")
	// 	// height := int64(4) // The block height you're interested in

	// 	cosmosClient, err := CreateCosmosClient(cometBFTClient)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}

	// 	// Get the proof
	// 	res, err := cosmosClient.Client.ABCIQueryWithOptions(context.Background(), "/store/hashed_accounts/key", k, client.ABCIQueryOptions{
	// 		Height: block.Block.Height,
	// 		Prove:  true,
	// 	})
	// 	fmt.Println("proof call completed")
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}

	// 	fmt.Printf("ABCI QUERY: %+v\n", res)
	// }
	// app.rwMutex.RLock()
	// defer app.rwMutex.RUnlock()

	// rpcEndpoint := "http://localhost:26657" // Replace with your node's RPC endpoint
	// cometBFTClient, err := CreateCometBFTClient(rpcEndpoint)
	// if err != nil {
	// 	return nil, err
	// }
	// defer cometBFTClient.Stop() // Remember to stop the client when you're done

	// // Now you can use the client to interact with your CometBFT node
	// // For example, to get the latest block:
	// block, err := cometBFTClient.Block(context.Background(), nil)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Printf("BLOCK: %+v\n", block)
	// // fmt.Printf("Latest block height: %d\n", block.Block.Height)

	// if block.Block != nil && block.Block.Height > 2 {
	// 	fmt.Println("HERE")
	// 	// The block height you're interested in
	// 	cosmosClient, err := app.CreateCosmosClient(cometBFTClient, block.Block.Height)
	// 	if err != nil {
	// 		fmt.Println("----------")
	// 		return nil, err
	// 	}
	// 	// context, err := app.CreateQueryContext(block.Block.Height, true)
	// 	// if err != nil {
	// 	// 	return nil, err
	// 	// }

	// 	// fmt.Println("?????????????")
	// 	block, err := cosmosClient.Client.Block(context.Background(), nil)
	// 	fmt.Println("______________")
	// 	if err != nil {
	// 		fmt.Println("eeeeeeeeee")
	// 		return nil, err
	// 	}

	// 	fmt.Println("ffffffffffff")
	// 	fmt.Printf("HERE BLOCK: %+v\n", block)

	// 	fmt.Println("@@@@@@@@@@@@@@")

	// res, err := cosmosClient.Client.ABCIQueryWithOptions(context.Background(), "/store/hashed_storages/key", []byte{1}, client.ABCIQueryOptions{
	// 	Prove: true,
	// })
	// abciReq := abci.RequestQuery{
	// 	Path:   "/store/hashed_stoages/key",
	// 	Height: 0,
	// 	Data:   []byte{1},
	// 	Prove:  true,
	// }
	// res, err := cosmosClient.QueryABCI(abciReq)
	// fmt.Println("2222222222")
	// if err != nil {
	// 	fmt.Println("///////////")
	// 	return nil, err
	// }

	// 	value, proof, err := GetProof(cosmosClient, "hashed_storages", []byte{1})
	// 	if err != nil {
	// 		fmt.Println("ERROR: ", err)
	// 		fmt.Println("(((((((())))))))")
	// 		return nil, err
	// 	}

	// 	fmt.Printf("PROOF: %+v: %+v\n", value, proof)
	// }
	// fmt.Println("ITERATOR POSITION BEFORE SEEK BY KEY SUBKEY: ", evm.EthIteratorsCurrentKey[cursorID])

	// rpcEndpoint := "http://localhost:26657" // Replace with your node's RPC endpoint
	// cometBFTClient, err := CreateCometBFTClient(rpcEndpoint)
	// if err != nil {
	// 	return nil, err
	// }
	// defer cometBFTClient.Stop() // Remember to stop the client when you're done

	// // Now you can use the client to interact with your CometBFT node
	// // For example, to get the latest block:
	// block, err := cometBFTClient.Block(context.Background(), nil)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Printf("BLOCK: %+v\n", block)
	// // fmt.Printf("Latest block height: %d\n", block.Block.Height)

	// if block.Block != nil && block.Block.Height > 2 {

	// 	// key := []byte("hashed_storages")
	// 	// height := int64(4) // The block height you're interested in

	// 	cosmosClient, err := app.CreateCosmosClient(cometBFTClient)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	// value, proof, err := GetProof(cosmosClient, "hashed_storages", []byte{1})
	// 	// if err != nil {
	// 	// 	return nil, err
	// 	// }

	// 	// Get the proof
	// 	// res, err := cosmosClient.Client.ABCIQueryWithOptions(context.Background(), "/store/hashed_storages/key", key, client.ABCIQueryOptions{
	// 	// 	Height: block.Block.Height,
	// 	// 	Prove:  true,
	// 	// })
	// 	// if err != nil {
	// 	// 	log.Fatal(err)
	// 	// }

	// 	fmt.Println("EEEEEEE")
	// 	res, err := cosmosClient.Client.ABCIQueryWithOptions(context.Background(), "/store/hashed_a/key", key, client.ABCIQueryOptions{
	// 		Height: block.Block.Height,
	// 		Prove:  true,
	// 	})
	// 	fmt.Println("RRRRRRR")
	// 	if err != nil {

	// 		log.Fatal(err)
	// 	}

	// 	fmt.Printf("ABCI QUERY: %+v\n", res)

	// 	// fmt.Printf("ABCI QUERY: %+v : : %+v\n", value, proof)
	// }

	table, err := app.getTable(tableCode)
	if err != nil {
		return nil, err
	}

	if !table.DupSorted {
		return nil, ErrInvalidRequestData
	}

	iterator := app.executionCacheMultistore.GetKVStore(table.StoreKey).Iterator(key, nil)
	defer iterator.Close()

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()

	if !iterator.Valid() {
		// fmt.Println("KEYSUBKEY CASE 1")
		delete(evm.EthIteratorsCurrentKey, cursorID)
		return nil, nil
	}

	evm.EthIteratorsCurrentKey[cursorID] = iterator.Key()
	// fmt.Println("ITERATOR POSITION AFTER SEEK BY KEY SUBKEY: ", evm.EthIteratorsCurrentKey[cursorID])

	if !bytes.HasPrefix(iterator.Key(), key[:table.KeyBytes]) {
		// fmt.Println("KEYSUBKEY CASE 2")
		return nil, nil
	}

	// fmt.Println("KEYSUBKEY CASE 3")

	response := append(iterator.Key()[table.KeyBytes:], iterator.Value()...)
	return response, nil
}

// DeleteCurrentDuplicates: deletes all entries for current key(not key+subkey)
// if their is not next key, then unset the cursor
// fail if cursor is not set
func (app *ScalerizeApp) DeleteCurrentDuplicates(tableCode uint8, cursorID [8]byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("ITERATOR POSITION BEFORE DELETE CURRENT DUPLICATES: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	if !table.DupSorted {
		return ErrInvalidRequestData
	}

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	currentKey, ok := evm.EthIteratorsCurrentKey[cursorID]
	if !ok {
		return ErrCurrentKeyIsNotSet
	}

	key := currentKey[:table.KeyBytes]
	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)

	iterator := store.Iterator(key, storetypes.PrefixEndBytes(key))
	defer iterator.Close()

	if !iterator.Valid() {
		return ErrCurrentIteratorKeyIsInvalid
	}

	for ; iterator.Valid(); iterator.Next() {
		store.Delete(iterator.Key())
	}

	nextIterator := store.Iterator(storetypes.PrefixEndBytes(key), nil)
	defer nextIterator.Close()

	if !nextIterator.Valid() {
		delete(evm.EthIteratorsCurrentKey, cursorID)
	} else {
		evm.EthIteratorsCurrentKey[cursorID] = nextIterator.Key()
	}

	// fmt.Println("ITERATOR POSITION AFTER DELETE CURRENT DUPLICATES: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}

// AppendDup: appends new entry in the table
// if key exists: the subkey specified should be equal to or more than greatest subkey for that key
// if key not exists: just add the new entry
func (app *ScalerizeApp) AppendDup(tableCode uint8, cursorID [8]byte, k, value []byte) error {
	app.rwMutex.Lock()
	defer app.rwMutex.Unlock()

	// fmt.Println("ITERATOR POSITION BEFORE APPEND DUP: ", evm.EthIteratorsCurrentKey[cursorID])

	table, err := app.getTable(tableCode)
	if err != nil {
		return err
	}

	if !table.DupSorted {
		return ErrInvalidRequestData
	}

	key := k[:table.KeyBytes]
	subkey := k[table.KeyBytes:]
	store := app.executionCacheMultistore.GetKVStore(table.StoreKey)

	iterator := store.ReverseIterator(key, storetypes.PrefixEndBytes(key))
	defer iterator.Close()

	evm.EthIteratorsCurrentKeyLock.Lock()
	defer evm.EthIteratorsCurrentKeyLock.Unlock()
	if !iterator.Valid() {
		// fmt.Println("APPEND DUP CASE 1")
		store.Set(k, value)
		evm.EthIteratorsCurrentKey[cursorID] = k
	} else {
		greatestSubkey := iterator.Key()[table.KeyBytes:]
		// fmt.Println("SUBKEY: ", subkey)
		// fmt.Println("GREATEST SUBKEY: ", greatestSubkey)

		if bytes.Compare(subkey, greatestSubkey) < 0 {
			// fmt.Println("APPEND DUP CASE 2")
			return ErrCannotAppendDupIfSubkeyIsLessThanGreatestSubKeyForKey
		}

		// fmt.Println("COMPARE: ", bytes.Compare(subkey, greatestSubkey))
		// fmt.Println("APPEND DUP CASE 3")
		store.Set(k, value)

		evm.EthIteratorsCurrentKey[cursorID] = k
	}

	// fmt.Println("ITERATOR POSITION AFTER APPEND DUP: ", evm.EthIteratorsCurrentKey[cursorID])

	return nil
}
