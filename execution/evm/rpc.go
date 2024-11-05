package evm

import (
	"strconv"

	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

func (ec *EVMClient) GetLatestBlockNumber() (rpc.BlockNumber, error) {
	var result string
	if err := ec.rpcClient.Client().CallContext(ec.ctx, &result, BlockNumberMethod); err != nil {
		return 0, err
	}

	num, err := strconv.ParseInt(result, 0, 64)
	if err != nil {
		return 0, err
	}

	return rpc.BlockNumber(num), nil
}

func (ec *EVMClient) GetBlockByNumber(num rpc.BlockNumber, withTxs bool) (*coretypes.Header, error) {
	result := &coretypes.Header{}
	err := ec.rpcClient.Client().CallContext(ec.ctx, result, BlockByNumberMethod, num, withTxs)
	return result, err
}
