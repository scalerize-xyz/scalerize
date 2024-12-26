package app

import (
	storetypes "cosmossdk.io/store/types"
)

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

// todo: use an array instead of a map
var (
	lookUpTable = map[uint8]TableInfo{
		StoreHeaderNumbers: {
			NoOfKeyBytes: 1,
			StoreKey:     storetypes.NewKVStoreKey("HeaderNumbers"),
		},
	}
)
