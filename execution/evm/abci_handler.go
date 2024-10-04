package evm

import (
	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ABCIHandler struct {
	client EVMClient
}

func NewEVMABCIHandler() *ABCIHandler {
	return &ABCIHandler{
		// client: client,
	}
}

func (h *ABCIHandler) PrepareProposal() sdk.PrepareProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		prepareProposalMockResponse := `
			{
				"parentHash": "0x1ecdf28cea1886cee4b560ae85bc5e41f675646f6e7d21c6b8214fdf917da360",
				"feeRecipient": "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
				"stateRoot": "0xe7d2096d708957a50a144c303dfb69cfcf9d3182ab3a9d52a76e592fcde11109",
				"receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"prevRandao": "0x0000000000000000000000000000000000000000000000000000000000000000",
				"blockNumber": "0x1",
				"gasLimit": "0x1c9c380",
				"gasUsed": "0x0",
				"timestamp": "0x66fe8cd8",
				"extraData": "0x726574682f76312e302e372f6c696e7578",
				"baseFeePerGas": "0x342770c0",
				"blockHash": "0xbcd50aec609f2613ac63d51eb87a89c2d6d15800f2d87ae3c56ab4ad1c6b5da5",
				"transactions": [],
				"withdrawals": [],
				"blobGasUsed": "0x0",
				"excessBlobGas": "0x0"
				},
		}`

		return &abci.ResponsePrepareProposal{
			Txs: [][]byte{[]byte(prepareProposalMockResponse)},
		}, nil
	}
}

func (h *ABCIHandler) ProcessProposal() sdk.ProcessProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		return nil, nil
	}
}

func (h *ABCIHandler) PreBlock() sdk.PreBlocker {
	return func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
		return nil, nil
	}
}

func (h *ABCIHandler) EndBlock() sdk.EndBlocker {
	return func(ctx sdk.Context) (sdk.EndBlock, error) {
		return sdk.EndBlock{}, nil
	}
}
