package evm

import (
	"context"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/core/types"
)

type EVMABCIHandler struct {
	ctx    context.Context
	client *EVMClient
}

func NewEVMABCIHandler(ctx context.Context, evmClient *EVMClient) (*EVMABCIHandler, error) {
	return &EVMABCIHandler{
		ctx:    ctx,
		client: evmClient,
	}, nil
}

func (h *EVMABCIHandler) PrepareProposal() sdk.PrepareProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		// 1. getlatestblock number
		lbn, err := h.client.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}

		fmt.Printf("LATEST BLOCK NUMBER: %v\n", lbn.Int64())

		// 2. get block by number
		bh, err := h.client.GetBlockByNumber(lbn, false)
		if err != nil {
			return nil, err
		}

		fmt.Printf("LATEST BLOCK HEADER: %+v\n", bh)

		// 2.5 use jwt generator to generate token
		//done in client formation

		// 3. Prepare request for fork choice update
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

		fcres, err := h.client.ForkchoiceUpdated(state, attr)
		if err != nil {
			return nil, err
		}

		fmt.Printf("ForkchoiceUpdated response: %+v\n", fcres)

		// 4. Recieve the payload ID and prepare request for get payload
		payload, err := h.client.GetPayload(*fcres.PayloadID)
		if err != nil {
			return nil, err
		}

		fmt.Printf("PAYLOAD: %+v\n", payload)

		// Note: put retries in 3and 4 and use JWT generator in the same

		// 5. get payload and make a prepareProposal response

		return &abci.ResponsePrepareProposal{
			Txs: [][]byte{},
		}, nil
	}
}

func (h *EVMABCIHandler) ProcessProposal() sdk.ProcessProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		fmt.Printf("Process Proposal Request: %+v\n", req)
		// Once you receive the prepare proposal response make a new payload request to the EVM.
		return &abci.ResponseProcessProposal{
			Status: abci.ResponseProcessProposal_ACCEPT,
		}, nil
	}
}

func (h *EVMABCIHandler) PreBlock() sdk.PreBlocker {
	return func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
		return &sdk.ResponsePreBlock{
			ConsensusParamsChanged: false,
		}, nil
	}
}

func (h *EVMABCIHandler) EndBlock() sdk.EndBlocker {
	return func(ctx sdk.Context) (sdk.EndBlock, error) {
		return sdk.EndBlock{}, nil
	}
}
