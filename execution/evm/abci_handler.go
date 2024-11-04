package evm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
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
		// todo: put retries for rpc and engine api calls

		lbn, err := h.client.GetLatestBlockNumber()
		if err != nil {
			return nil, err
		}

		fmt.Printf("LATEST BLOCK NUMBER: %v\n", lbn.Int64())

		bh, err := h.client.GetBlockByNumber(lbn, false)
		if err != nil {
			return nil, err
		}

		fmt.Printf("LATEST BLOCK HEADER: %+v\n", bh)
		fmt.Println("LATEST BLOCK HASH: ", bh.Hash())

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

		payloadExData, err := h.client.GetPayload(*fcres.PayloadID)
		if err != nil {
			return nil, err
		}

		fmt.Printf("PAYLOAD EXECUTABLE DATA: %+v\n", payloadExData)
		fmt.Printf("EXECUTION PAYLOAD: %+v\n", payloadExData.ExecutionPayload)
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

func (h *EVMABCIHandler) ProcessProposal() sdk.ProcessProposalHandler {
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

		fmt.Printf("RECIEVED EXECUTION PAYLOAD: %+v\n", executableData)
		fmt.Printf("RECIEVED PAYLOAD ATTRIBUTES: %+v\n", attributes)

		res, err := h.client.NewPayload(*executableData, []common.Hash{}, (common.Hash)(attributes.ParentBeaconBlockRoot))
		if err != nil {
			return nil, err
		}

		fmt.Printf("NEW PAYLOAD RESULT: %+v\n", res)

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
