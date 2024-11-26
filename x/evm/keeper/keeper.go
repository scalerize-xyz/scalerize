package keeper

import (
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
)

type Keeper struct {
	storeKey storetypes.StoreKey
	cdc      codec.BinaryCodec
}

func NewKeeper(storeKey storetypes.StoreKey, cdc codec.BinaryCodec) *Keeper {
	return &Keeper{
		storeKey: storeKey,
		cdc:      cdc,
	}
}
