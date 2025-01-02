package app

import (
	storetypes "cosmossdk.io/store/types"
)

const NumberOfTables uint8 = 3

type TableInfo struct {
	NoOfKeyBytes int
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
