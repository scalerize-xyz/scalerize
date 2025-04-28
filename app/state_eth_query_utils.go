package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	abci "github.com/cometbft/cometbft/abci/types"
	crypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cometbft/cometbft/rpc/client/http"
	cosmossdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
)

// AccountResult is the result of a GetProof operation.
type AccountResult struct {
	Address      common.Address  `json:"address"`
	AccountProof []string        `json:"accountProof"`
	Balance      *big.Int        `json:"balance"`
	CodeHash     common.Hash     `json:"codeHash"`
	Nonce        uint64          `json:"nonce"`
	StorageHash  common.Hash     `json:"storageHash"`
	StorageProof []StorageResult `json:"storageProof"`
}

// StorageResult provides a proof for a key-value pair.
type StorageResult struct {
	Key   string       `json:"key"`
	Value *uint256.Int `json:"value"`
	Proof []string     `json:"proof"`
}

type BlockNumberOrHash struct {
	BlockNumber *int64       `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash `json:"blockHash,omitempty"`
}

func createCometBFTClient(rpcEndpoint string) (*http.HTTP, error) {
	client, err := http.New(rpcEndpoint, "/websocket")
	if err != nil {
		return nil, err
	}

	// Optionally, you can start the client
	err = client.Start()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func createCosmosClient(cometBFTClient *http.HTTP) (cosmossdkclient.Context, error) {
	interfaceRegistry := types.NewInterfaceRegistry()
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	clientCtx := cosmossdkclient.Context{}.
		WithClient(cometBFTClient).
		WithCodec(marshaler).
		WithInterfaceRegistry(interfaceRegistry)

	return clientCtx, nil
}

// in our case we will get the storage slot directly and
// will populate key in StorageResult, balance, codeHash and none in AccountResult on reth side
// need to send the bincode serialized hashed address and hashed storageKeys
func getProof(cometBFTClient *http.HTTP, serializedHashedAccountAddress []byte, serializedStorageKeys [][]byte, blockNumOrHash *BlockNumberOrHash) (*AccountResult, error) {
	fmt.Println("serializedHashedAccountAddress: ", serializedHashedAccountAddress)
	fmt.Println("serializedHashedStorageKeys: ", serializedStorageKeys)
	fmt.Printf("blockNumOrHash: %+v\n", blockNumOrHash)

	blockNumber, err := blockNumberFromTendermint(cometBFTClient, *blockNumOrHash)
	if err != nil {
		return nil, err
	}

	fmt.Println("PROOF BLOCK NUMBER: ", blockNumber)

	// query storage proofs
	storageProofs := make([]StorageResult, len(serializedStorageKeys))

	cosmosClient, err := createCosmosClient(cometBFTClient)
	if err != nil {
		return nil, err
	}

	cosmosClient = cosmosClient.WithHeight(blockNumber)

	for i, key := range serializedStorageKeys {
		// hexKey := common.HexToHash(key)
		valueBz, proof, err := getProofForKey(cosmosClient, HashedStoragesStoreName, append(serializedHashedAccountAddress, key...))
		if err != nil {
			return nil, err
		}

		fmt.Println("STORAGE PROOF VAL BYTES: ", valueBz)

		fmt.Println("HEX STRING", hex.EncodeToString(key))
		storageProofs[i] = StorageResult{
			Key:   hex.EncodeToString(key[8:]),
			Value: uint256.NewInt(0).SetBytes(valueBz),
			Proof: getHexProofs(proof),
		}
	}

	// query account proofs
	// hashedAccountAddress := gethcrypto.Keccak256Hash(address)
	accountVal, proof, err := getProofForKey(cosmosClient, HashedAccountsStoreName, serializedHashedAccountAddress)
	if err != nil {
		return nil, err
	}

	fmt.Println("ACCOUNT PROOF VAL BYTES: ", accountVal)

	return &AccountResult{
		Address:      common.Address{},
		AccountProof: getHexProofs(proof),
		Balance:      (new(big.Int).SetBytes(make([]byte, 32))),
		Nonce:        0,
		StorageHash:  common.Hash{},
		StorageProof: storageProofs,
	}, nil
}

func getHexProofs(proof *crypto.ProofOps) []string {
	if proof == nil {
		return []string{""}
	}
	proofs := []string{}
	// check for proof
	for _, p := range proof.Ops {
		proof := ""
		if len(p.Data) > 0 {
			proof = hexutil.Encode(p.Data)
		}
		proofs = append(proofs, proof)
	}
	return proofs
}

func getProofForKey(clientCtx cosmossdkclient.Context, storeKey string, key []byte) ([]byte, *crypto.ProofOps, error) {
	height := clientCtx.Height
	// ABCI queries at height less than or equal to 2 are not supported.
	// Base app does not support queries for height less than or equal to 1.
	// Therefore, a query at height 2 would be equivalent to a query at height 3
	if height <= 2 {
		return nil, nil, fmt.Errorf("proof queries at height <= 2 are not supported")
	}

	abciReq := abci.RequestQuery{
		Path:   fmt.Sprintf("store/%s/key", storeKey),
		Data:   key,
		Height: height,
		Prove:  true,
	}

	abciRes, err := clientCtx.QueryABCI(abciReq)
	if err != nil {
		return nil, nil, err
	}

	// fmt.Printf("PROOF RESPONSE: %+v\n", abciRes)

	return abciRes.Value, abciRes.ProofOps, nil
}

func blockNumberFromTendermint(cometbftClient *http.HTTP, blockNrOrHash BlockNumberOrHash) (int64, error) {
	switch {
	case blockNrOrHash.BlockHash == nil && blockNrOrHash.BlockNumber == nil:
		return 0, fmt.Errorf("types BlockHash and BlockNumber cannot be both nil")
	case blockNrOrHash.BlockHash != nil:
		block, err := cometbftClient.BlockByHash(context.Background(), blockNrOrHash.BlockHash.Bytes())
		if err != nil {
			return 0, err
		}
		return block.Block.Height, nil
	case blockNrOrHash.BlockNumber != nil:
		if *blockNrOrHash.BlockNumber == -1 {
			block, err := cometbftClient.Block(context.Background(), nil)
			if err != nil {
				return 0, err
			}

			return block.Block.Height, nil
		}

		return *blockNrOrHash.BlockNumber, nil
	default:
		return 0, nil
	}
}
