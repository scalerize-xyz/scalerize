package app

import (
	storetypes "cosmossdk.io/store/types"
)

const NumberOfTables uint8 = 3

type TableInfo struct {
	NoOfKeyBytes int
	StoreKey     storetypes.StoreKey
}

const (
	StoreCanonicalHeaders = iota
	StoreHeaderTerminalDifficulties
	StoreHeaderNumbers
	StoreHeaders
	StoreBlockOmmers
	StoreBlockWithdrawals
)

var (
	lookUpTable = [NumberOfTables]TableInfo{
		{
			NoOfKeyBytes: 1,
			StoreKey:     storetypes.NewKVStoreKey("CanonicalHeaders"),
		},
		{
			NoOfKeyBytes: 1,
			StoreKey:     storetypes.NewKVStoreKey("HeaderTerminalDifficulties"),
		},
		StoreHeaderNumbers: {
			NoOfKeyBytes: 1,
			StoreKey:     storetypes.NewKVStoreKey("HeaderNumbers"),
		},
	}
)
