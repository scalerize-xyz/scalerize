package types

import (
	storetypes "cosmossdk.io/store/types"
)

const (
	ModuleName = "evm"
	StoreKey   = ModuleName
	RouterKey  = ModuleName
)

var (
	EVMStoreKey = storetypes.NewKVStoreKey(StoreKey)
)
