package evm

import (
	"encoding/json"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func (c *EVMClient) PrepareProposal() sdk.PrepareProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		// store := c.app.CommitMultiStore().GetKVStore(storetypes.NewKVStoreKey("hashed_accounts"))
		// iterator := store.Iterator(nil, nil) // This will iterate over all keys
		// defer iterator.Close()

		// fmt.Println("All data in store: hashed_accounts")
		// for ; iterator.Valid(); iterator.Next() {
		// 	key := iterator.Key()
		// 	value := iterator.Value()
		// 	fmt.Printf("Key: %x, Value: %x\n", key, value)
		// }

		// store = c.app.CommitMultiStore().GetKVStore(storetypes.NewKVStoreKey("hashed_storages"))
		// iterator = store.Iterator(nil, nil) // This will iterate over all keys
		// defer iterator.Close()

		// fmt.Println("All data in store: hashed_storages")
		// for ; iterator.Valid(); iterator.Next() {
		// 	key := iterator.Key()
		// 	value := iterator.Value()
		// 	fmt.Printf("Key: %x, Value: %x\n", key, value)
		// }

		// todo: put retries for rpc and engine api calls

		// the store reflects the changes made through the web server created for crud operations in multistore
		// params := evmtypes.Params{}
		// kvstore := ctx.KVStore(evmtypes.EVMStoreKey)
		// bz := kvstore.Get([]byte{3})
		// json.Unmarshal(bz, &params)
		// fmt.Printf("PARAMS IN PREPARE PROPOSAL: %+v\n", params)

		lbn, err := c.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}

		// fmt.Printf("LATEST BLOCK NUMBER: %v\n", lbn.Int64())

		bh, err := c.GetBlockByNumber(lbn, false)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("LATEST BLOCK HEADER: %+v\n", bh)
		// fmt.Println("LATEST BLOCK HASH: ", bh.Hash())

		state := &ForkchoiceState{
			HeadBlockHash:      bh.Hash(),
			SafeBlockHash:      bh.ParentHash,
			FinalizedBlockHash: bh.ParentHash,
		}

		randao, err := generateRandao()
		if err != nil {
			return nil, err
		}

		sfr, err := hexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
		if err != nil {
			return nil, err
		}

		attr := PayloadAttributes{
			Timestamp:             uint64(time.Now().Unix()),
			Random:                randao,
			SuggestedFeeRecipient: sfr,
			ParentBeaconBlockRoot: *bh.ParentBeaconRoot,
			Withdrawals:           []*types.Withdrawal{},
		}

		fcres, err := c.ForkchoiceUpdated(state, attr)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("ForkchoiceUpdated response: %+v\n", fcres)

		time.Sleep(10 * time.Millisecond)

		payloadExData, err := c.GetPayload(*fcres.PayloadID)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("PAYLOAD EXECUTABLE DATA: %+v\n", payloadExData)
		// fmt.Printf("EXECUTION PAYLOAD: %+v\n", payloadExData.ExecutionPayload)
		fmt.Printf("APP HASH PREPARE PROPOSAL: %+v\n", payloadExData.ExecutionPayload.StateRoot)
		pb, err := payloadExData.ExecutionPayload.MarshalJSON()
		if err != nil {
			return nil, err
		}

		ab, err := json.Marshal(attr)
		if err != nil {
			return nil, err
		}

		return &abci.ResponsePrepareProposal{
			Txs: [][]byte{pb, ab},
		}, nil
	}
}

func (c *EVMClient) ProcessProposal() sdk.ProcessProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		// Once you receive the prepare proposal response make a new payload request to the EVM.
		var (
			executableData = &ExecutableData{}
			attributes     = &PayloadAttributes{}
		)

		if err := executableData.UnmarshalJSON(req.Txs[0]); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(req.Txs[1], attributes); err != nil {
			return nil, err
		}

		// fmt.Printf("RECIEVED EXECUTION PAYLOAD: %+v\n", executableData)
		// fmt.Printf("RECIEVED PAYLOAD ATTRIBUTES: %+v\n", attributes)

		_, err := c.NewPayload(*executableData, []common.Hash{}, (common.Hash)(attributes.ParentBeaconBlockRoot))
		if err != nil {
			return nil, err
		}

		// fmt.Printf("NEW PAYLOAD RESULT: %+v\n", res)

		return &abci.ResponseProcessProposal{
			Status: abci.ResponseProcessProposal_ACCEPT,
		}, nil
	}
}

func (c *EVMClient) PreBlock() sdk.PreBlocker {
	return func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
		executableData := &ExecutableData{}

		if err := executableData.UnmarshalJSON(req.Txs[0]); err != nil {
			return nil, err
		}
		state := &ForkchoiceState{
			HeadBlockHash:      executableData.BlockHash,
			SafeBlockHash:      executableData.ParentHash,
			FinalizedBlockHash: executableData.ParentHash,
		}

		_, err := c.ForkchoiceUpdated(state, nil)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("PRE BLOCK ForkchoiceUpdated response: %+v\n", fcres)

		return &sdk.ResponsePreBlock{
			ConsensusParamsChanged: false,
		}, nil
	}
}

func (c *EVMClient) EndBlock() sdk.EndBlocker {
	return func(ctx sdk.Context) (sdk.EndBlock, error) {
		return sdk.EndBlock{}, nil
	}
}
