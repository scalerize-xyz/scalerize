package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	PayloadStatusValid    string = "VALID"
	PayloadStatusInvalid  string = "INVALID"
	PayloadStatusSyncing  string = "SYNCING"
	PayloadStatusAccepted string = "ACCEPTED"
)

type PayloadID string

type ForkchoiceState struct {
	// Block hash of the head of the canonical chain.
	HeadBlockHash common.Hash `json:"headBlockHash"`
	// "Safe" block hash of the canonical chain under certain
	// synchrony and honesty assumptions. This value MUST be either
	// equal to or an ancestor of headBlockHash
	SafeBlockHash common.Hash `json:"safeBlockHash"`
	// Block hash of the most recent finalized block
	FinalizedBlockHash common.Hash `json:"finalizedBlockHash"`
}

type ForkchoiceResponse struct {
	PayloadStatus PayloadStatus `json:"payloadStatus"`
	// identifier of the payload build process or null
	PayloadID *PayloadID `json:"payloadId"`
}

type PayloadStatus struct {
	// Either "VALID", "INVALID", "SYNCING","ACCEPTED",
	// "INVALID_BLOCK_HASH", or "INVALID_TERMINAL_BLOCK"
	Status string `json:"status"`
	// Hash of the most recent valid block in the branch
	// defined by payload and its ancestors
	LatestValidHash *common.Hash `json:"latestValidHash"`
	// Message providing additional details on the validation
	// error if the payload is classified as INVALID, INVALID_BLOCK_HASH
	// or INVALID_TERMINAL_BLOCK
	ValidationError *string `json:"validationError"`
}

type ExecutionPayloadEnvelope struct {
	ExecutionPayload *ExecutableData `json:"executionPayload"`
	BlockValue       *big.Int        `json:"blockValue"`
}

// ExecutableData is the data necessary to execute an EL payload.
type ExecutableData struct {
	ParentHash    common.Hash         `json:"parentHash"`
	FeeRecipient  common.Address      `json:"feeRecipient"`
	StateRoot     common.Hash         `json:"stateRoot"`
	ReceiptsRoot  common.Hash         `json:"receiptsRoot"`
	LogsBloom     []byte              `json:"logsBloom"`
	Random        common.Hash         `json:"prevRandao"`
	Number        uint64              `json:"blockNumber"`
	GasLimit      uint64              `json:"gasLimit"`
	GasUsed       uint64              `json:"gasUsed"`
	Timestamp     uint64              `json:"timestamp"`
	ExtraData     []byte              `json:"extraData"`
	BaseFeePerGas string              `json:"baseFeePerGas"`
	BlockHash     common.Hash         `json:"blockHash"`
	Transactions  [][]byte            `json:"transactions"`
	Withdrawals   []*types.Withdrawal `json:"withdrawals"`
	BlobGasUsed   uint64              `json:"blobGasUsed"`
	ExcessBlobGas uint64              `json:"excessBlobGas"`
}

type PayloadAttributes struct {
	Timestamp             uint64              `json:"timestamp"`
	Random                common.Hash         `json:"prevRandao"`
	SuggestedFeeRecipient common.Address      `json:"suggestedFeeRecipient"`
	Withdrawals           []*types.Withdrawal `json:"withdrawals"`
	ParentBeaconBlockRoot common.Hash         `json:"parentBeaconBlockRoot"`
}
