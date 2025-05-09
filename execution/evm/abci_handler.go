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
		fmt.Println("PREPARE PROPOSAL")
		if c.syncStatus == nil {
			c.syncStatus = &SyncStatus{syncing: false}
		}

		lbn, err := c.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}

		bh, err := c.GetBlockByNumber(lbn, false)
		if err != nil {
			return nil, err
		}

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

		time.Sleep(20 * time.Millisecond)

		payloadExData, err := c.GetPayload(*fcres.PayloadID)
		if err != nil {
			return nil, err
		}

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
		fmt.Println("PROCESS PROPOSAL")
		if c.syncStatus == nil {
			c.syncStatus = &SyncStatus{syncing: false}
		}

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

		_, err := c.NewPayload(*executableData, []common.Hash{}, (common.Hash)(attributes.ParentBeaconBlockRoot))
		if err != nil {
			return nil, err
		}

		return &abci.ResponseProcessProposal{
			Status: abci.ResponseProcessProposal_ACCEPT,
		}, nil
	}
}

func (c *EVMClient) PreBlock() sdk.PreBlocker {
	return func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
		fmt.Println("PREBLOCK")
		if c.syncStatus == nil {
			c.syncStatus = &SyncStatus{syncing: true}
		}

		fmt.Printf("SYNCING: %+v\n", c.syncStatus)
		if c.syncStatus.syncing {
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

			_, err := c.NewPayload(*executableData, []common.Hash{}, (common.Hash)(attributes.ParentBeaconBlockRoot))
			if err != nil {
				return nil, err
			}
		}
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

		EthIteratorsCurrentKeyLock.Lock()
		defer EthIteratorsCurrentKeyLock.Unlock()
		EthIteratorsCurrentKey = map[[8]byte][]byte{}

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
