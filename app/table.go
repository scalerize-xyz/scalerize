package app

import (
	storetypes "cosmossdk.io/store/types"
)

const NumberOfTables uint8 = 3

// need to mangage the key for:
// 1. DbCursorRO and 2. DbCursorRW for normal tables
// 3.  DbDupCursorRO and 4. DbDupCursorRW for dup sorted tables
type TableInfo struct {
	NoOfKeyBytes uint
	StoreKey     storetypes.StoreKey
	IteratorsKey []byte
}

const (
	StoreCanonicalHeaders = iota
	StoreHeaderTerminalDifficulties
	StoreHeaderNumbers
	StoreHeaders
	StoreBlockOmmers
	StoreBlockWithdrawals

	BytesForCanonicalHeadersKey           = 4
	BytesForHeaderTerminalDifficultiesKey = 4
	BytesForHeaderNumbersKey              = 4
)

var (
	lookUpTable = [NumberOfTables]TableInfo{
		{
			NoOfKeyBytes: BytesForCanonicalHeadersKey,
			StoreKey:     storetypes.NewKVStoreKey("CanonicalHeaders"),
		},
		{
			NoOfKeyBytes: BytesForHeaderTerminalDifficultiesKey,
			StoreKey:     storetypes.NewKVStoreKey("HeaderTerminalDifficulties"),
		},
		StoreHeaderNumbers: {
			NoOfKeyBytes: BytesForHeaderNumbersKey,
			StoreKey:     storetypes.NewKVStoreKey("HeaderNumbers"),
		},
	}
)
