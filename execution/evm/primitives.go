package evm

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/common/hexutil"
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
	BlobsBundle      *BlobsBundleV1  `json:"blobsBundle"`
	Override         bool            `json:"shouldOverrideBuilder"`
}

func (e ExecutionPayloadEnvelope) MarshalJSON() ([]byte, error) {
	type ExecutionPayloadEnvelope struct {
		ExecutionPayload *ExecutableData `json:"executionPayload"  gencodec:"required"`
		BlockValue       *hexutil.Big    `json:"blockValue"  gencodec:"required"`
		BlobsBundle      *BlobsBundleV1  `json:"blobsBundle"`
		Requests         []hexutil.Bytes `json:"executionRequests"`
		Override         bool            `json:"shouldOverrideBuilder"`
		Witness          *hexutil.Bytes  `json:"witness,omitempty"`
	}
	var enc ExecutionPayloadEnvelope
	enc.ExecutionPayload = e.ExecutionPayload
	enc.BlockValue = (*hexutil.Big)(e.BlockValue)
	enc.BlobsBundle = e.BlobsBundle
	enc.Override = e.Override
	return json.Marshal(&enc)
}

func (e *ExecutionPayloadEnvelope) UnmarshalJSON(input []byte) error {
	type ExecutionPayloadEnvelope struct {
		ExecutionPayload *ExecutableData `json:"executionPayload"  gencodec:"required"`
		BlockValue       *hexutil.Big    `json:"blockValue"  gencodec:"required"`
		BlobsBundle      *BlobsBundleV1  `json:"blobsBundle"`
		Requests         []hexutil.Bytes `json:"executionRequests"`
		Override         *bool           `json:"shouldOverrideBuilder"`
		Witness          *hexutil.Bytes  `json:"witness,omitempty"`
	}
	var dec ExecutionPayloadEnvelope
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.ExecutionPayload == nil {
		return errors.New("missing required field 'executionPayload' for ExecutionPayloadEnvelope")
	}
	e.ExecutionPayload = dec.ExecutionPayload
	if dec.BlockValue == nil {
		return errors.New("missing required field 'blockValue' for ExecutionPayloadEnvelope")
	}
	e.BlockValue = (*big.Int)(dec.BlockValue)
	if dec.BlobsBundle != nil {
		e.BlobsBundle = dec.BlobsBundle
	}
	if dec.Override != nil {
		e.Override = *dec.Override
	}
	return nil
}

type BlobsBundleV1 struct {
	Commitments []hexutil.Bytes `json:"commitments"`
	Proofs      []hexutil.Bytes `json:"proofs"`
	Blobs       []hexutil.Bytes `json:"blobs"`
}

type PayloadAttributes struct {
	Timestamp             uint64              `json:"timestamp"`
	Random                common.Hash         `json:"prevRandao"`
	SuggestedFeeRecipient common.Address      `json:"suggestedFeeRecipient"`
	Withdrawals           []*types.Withdrawal `json:"withdrawals"`
	ParentBeaconBlockRoot common.Hash         `json:"parentBeaconBlockRoot"`
}

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
	BaseFeePerGas *big.Int            `json:"baseFeePerGas"`
	BlockHash     common.Hash         `json:"blockHash"`
	Transactions  [][]byte            `json:"transactions"`
	Withdrawals   []*types.Withdrawal `json:"withdrawals"`
	BlobGasUsed   *uint64             `json:"blobGasUsed"`
	ExcessBlobGas *uint64             `json:"excessBlobGas"`
}

func (e ExecutableData) MarshalJSON() ([]byte, error) {
	type ExecutableData struct {
		ParentHash       common.Hash             `json:"parentHash"    gencodec:"required"`
		FeeRecipient     common.Address          `json:"feeRecipient"  gencodec:"required"`
		StateRoot        common.Hash             `json:"stateRoot"     gencodec:"required"`
		ReceiptsRoot     common.Hash             `json:"receiptsRoot"  gencodec:"required"`
		LogsBloom        hexutil.Bytes           `json:"logsBloom"     gencodec:"required"`
		Random           common.Hash             `json:"prevRandao"    gencodec:"required"`
		Number           hexutil.Uint64          `json:"blockNumber"   gencodec:"required"`
		GasLimit         hexutil.Uint64          `json:"gasLimit"      gencodec:"required"`
		GasUsed          hexutil.Uint64          `json:"gasUsed"       gencodec:"required"`
		Timestamp        hexutil.Uint64          `json:"timestamp"     gencodec:"required"`
		ExtraData        hexutil.Bytes           `json:"extraData"     gencodec:"required"`
		BaseFeePerGas    *hexutil.Big            `json:"baseFeePerGas" gencodec:"required"`
		BlockHash        common.Hash             `json:"blockHash"     gencodec:"required"`
		Transactions     []hexutil.Bytes         `json:"transactions"  gencodec:"required"`
		Withdrawals      []*types.Withdrawal     `json:"withdrawals"`
		BlobGasUsed      *hexutil.Uint64         `json:"blobGasUsed"`
		ExcessBlobGas    *hexutil.Uint64         `json:"excessBlobGas"`
		ExecutionWitness *types.ExecutionWitness `json:"executionWitness,omitempty"`
	}
	var enc ExecutableData
	enc.ParentHash = e.ParentHash
	enc.FeeRecipient = e.FeeRecipient
	enc.StateRoot = e.StateRoot
	enc.ReceiptsRoot = e.ReceiptsRoot
	enc.LogsBloom = e.LogsBloom
	enc.Random = e.Random
	enc.Number = hexutil.Uint64(e.Number)
	enc.GasLimit = hexutil.Uint64(e.GasLimit)
	enc.GasUsed = hexutil.Uint64(e.GasUsed)
	enc.Timestamp = hexutil.Uint64(e.Timestamp)
	enc.ExtraData = e.ExtraData
	enc.BaseFeePerGas = (*hexutil.Big)(e.BaseFeePerGas)
	enc.BlockHash = e.BlockHash
	if e.Transactions != nil {
		enc.Transactions = make([]hexutil.Bytes, len(e.Transactions))
		for k, v := range e.Transactions {
			enc.Transactions[k] = v
		}
	}
	enc.Withdrawals = e.Withdrawals
	enc.BlobGasUsed = (*hexutil.Uint64)(e.BlobGasUsed)
	enc.ExcessBlobGas = (*hexutil.Uint64)(e.ExcessBlobGas)
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (e *ExecutableData) UnmarshalJSON(input []byte) error {
	type ExecutableData struct {
		ParentHash       *common.Hash            `json:"parentHash"    gencodec:"required"`
		FeeRecipient     *common.Address         `json:"feeRecipient"  gencodec:"required"`
		StateRoot        *common.Hash            `json:"stateRoot"     gencodec:"required"`
		ReceiptsRoot     *common.Hash            `json:"receiptsRoot"  gencodec:"required"`
		LogsBloom        *hexutil.Bytes          `json:"logsBloom"     gencodec:"required"`
		Random           *common.Hash            `json:"prevRandao"    gencodec:"required"`
		Number           *hexutil.Uint64         `json:"blockNumber"   gencodec:"required"`
		GasLimit         *hexutil.Uint64         `json:"gasLimit"      gencodec:"required"`
		GasUsed          *hexutil.Uint64         `json:"gasUsed"       gencodec:"required"`
		Timestamp        *hexutil.Uint64         `json:"timestamp"     gencodec:"required"`
		ExtraData        *hexutil.Bytes          `json:"extraData"     gencodec:"required"`
		BaseFeePerGas    *hexutil.Big            `json:"baseFeePerGas" gencodec:"required"`
		BlockHash        *common.Hash            `json:"blockHash"     gencodec:"required"`
		Transactions     []hexutil.Bytes         `json:"transactions"  gencodec:"required"`
		Withdrawals      []*types.Withdrawal     `json:"withdrawals"`
		BlobGasUsed      *hexutil.Uint64         `json:"blobGasUsed"`
		ExcessBlobGas    *hexutil.Uint64         `json:"excessBlobGas"`
		ExecutionWitness *types.ExecutionWitness `json:"executionWitness,omitempty"`
	}
	var dec ExecutableData
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for ExecutableData")
	}
	e.ParentHash = *dec.ParentHash
	if dec.FeeRecipient == nil {
		return errors.New("missing required field 'feeRecipient' for ExecutableData")
	}
	e.FeeRecipient = *dec.FeeRecipient
	if dec.StateRoot == nil {
		return errors.New("missing required field 'stateRoot' for ExecutableData")
	}
	e.StateRoot = *dec.StateRoot
	if dec.ReceiptsRoot == nil {
		return errors.New("missing required field 'receiptsRoot' for ExecutableData")
	}
	e.ReceiptsRoot = *dec.ReceiptsRoot
	if dec.LogsBloom == nil {
		return errors.New("missing required field 'logsBloom' for ExecutableData")
	}
	e.LogsBloom = *dec.LogsBloom
	if dec.Random == nil {
		return errors.New("missing required field 'prevRandao' for ExecutableData")
	}
	e.Random = *dec.Random
	if dec.Number == nil {
		return errors.New("missing required field 'blockNumber' for ExecutableData")
	}
	e.Number = uint64(*dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for ExecutableData")
	}
	e.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for ExecutableData")
	}
	e.GasUsed = uint64(*dec.GasUsed)
	if dec.Timestamp == nil {
		return errors.New("missing required field 'timestamp' for ExecutableData")
	}
	e.Timestamp = uint64(*dec.Timestamp)
	if dec.ExtraData == nil {
		return errors.New("missing required field 'extraData' for ExecutableData")
	}
	e.ExtraData = *dec.ExtraData
	if dec.BaseFeePerGas == nil {
		return errors.New("missing required field 'baseFeePerGas' for ExecutableData")
	}
	e.BaseFeePerGas = (*big.Int)(dec.BaseFeePerGas)
	if dec.BlockHash == nil {
		return errors.New("missing required field 'blockHash' for ExecutableData")
	}
	e.BlockHash = *dec.BlockHash
	if dec.Transactions == nil {
		return errors.New("missing required field 'transactions' for ExecutableData")
	}
	e.Transactions = make([][]byte, len(dec.Transactions))
	for k, v := range dec.Transactions {
		e.Transactions[k] = v
	}
	if dec.Withdrawals != nil {
		e.Withdrawals = dec.Withdrawals
	}
	if dec.BlobGasUsed != nil {
		e.BlobGasUsed = (*uint64)(dec.BlobGasUsed)
	}
	if dec.ExcessBlobGas != nil {
		e.ExcessBlobGas = (*uint64)(dec.ExcessBlobGas)
	}
	return nil
}
