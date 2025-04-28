package app

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/ethereum/go-ethereum/common"
)

func (app *ScalerizeApp) ethHandleStateConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("STARTING HANDLING CONNECTION")

	for {
		var (
			response []byte
		)

		// 1st byte contains the operation
		buffer := make([]byte, 4096)

		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Client closed connection")
			} else {
				app.Logger().Error("Connection error: " + err.Error())
			}
			return
		}

		if n == 0 {
			continue
		}

		data := buffer[:n]

		operation := data[0]
		// fmt.Println("OPERATION STATE: ", operation)

		// height := data[1:9]
		// fmt.Println("HEIGHT: ", height)

		switch operation {
		case OP_STATE_ROOT:
			if len(data) != 1+EthBlockNumberBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			height := data[1 : 1+EthBlockNumberBytes]
			// fmt.Println("HEIGHT: ", height)

			heightInt := int64(binary.BigEndian.Uint64(height))
			resp, err := app.StateRoot(heightInt)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(err.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}

		case OP_STATE_PROOF:
			// 2nd byte tells that BlockNumber(0) is given or BlockHash(1)
			var (
				storageKeys    [][]byte
				blockNumOrHash BlockNumberOrHash
				blockSpecBytes int
			)

			fmt.Println("DATA: ", data)
			if data[1] == 0 {
				bn := data[2 : 2+EthBlockNumberBytes]
				bnInt := int64(binary.BigEndian.Uint64(bn))
				blockNumOrHash.BlockNumber = &bnInt
				blockSpecBytes = EthBlockNumberBytes
			} else if data[1] == 1 {
				bn := -1
				bnInt := int64(bn)
				blockNumOrHash.BlockNumber = &bnInt
				blockSpecBytes = 0
			} else if data[1] == 2 {
				bh := data[2 : 2+EthBlockHashBytes]
				blockNumOrHash.BlockHash = &common.Hash{}
				copy(blockNumOrHash.BlockHash[:], bh)
				blockSpecBytes = EthBlockHashBytes
			} else {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			addressStart := 2 + blockSpecBytes
			addressEnd := addressStart + SerializedHashedAccountsKeyBytes
			// fmt.Println("ADDRESS START: ", addressStart)
			// fmt.Println("ADDRESS END: ", addressEnd)
			if len(data) < addressEnd {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}
			address := data[addressStart:addressEnd]

			if len(data) == addressEnd {
				storageKeys = [][]byte{}
			} else {
				// storageKeysBytes := len(data) - 1 + EthBlockNumberBytes + EthAccountAddressBytes
				combinedKeySize := SerializedHashedStoragesSubKeyBytes
				totalStorageBytes := len(data) - addressEnd
				// fmt.Println("COMBINED KEY SIZE: ", combinedKeySize)
				// fmt.Println("TotalStorageBytes ", totalStorageBytes)

				if totalStorageBytes%combinedKeySize != 0 {
					// fmt.Println("ERROR1")
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				numStorageKeys := totalStorageBytes / combinedKeySize
				for i := range numStorageKeys {
					start := addressEnd + i*combinedKeySize
					end := start + combinedKeySize
					storageKey := data[start:end]
					storageKeys = append(storageKeys, storageKey)
				}
			}

			// fmt.Printf("ACCOUNT: %+v\n", address)
			// for v := range storageKeys {
			// 	fmt.Printf("STORAGE KEY: %+v\n", v)
			// }

			resp, err := app.StateProof(&blockNumOrHash, address, storageKeys)
			if err != nil {
				// fmt.Println("ERROR2")
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}
		default:
			response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidOperationCode.Error())...)
		}

		// fmt.Println("RESPONSE STATE: ", response)
		app.writeToConn(conn, response)
	}
}

func (app *ScalerizeApp) StateRoot(height int64) ([]byte, error) {
	// fmt.Println("HEIGHT INT: ", height)
	if height < -1 {
		return nil, ErrInvalidRequestData
	}

	if height == -1 {
		// fmt.Println("RETURNING WORKING HASH: ", app.CommitMultiStore().WorkingHash())
		return app.CommitMultiStore().WorkingHash(), nil
	}

	cometBFTClient, err := createCometBFTClient(cometBFTRPCAddress)
	if err != nil {
		return nil, err
	}

	defer cometBFTClient.Stop()

	block, err := cometBFTClient.Block(context.Background(), &height)
	if err != nil {
		return nil, err
	}

	// fmt.Println("RETURNING HISTORICAL HASH: ", block.Block.AppHash)
	return block.Block.AppHash, nil
}

func (app *ScalerizeApp) StateProof(blockNumOrHash *BlockNumberOrHash, serializedHashedAccountAddress []byte, storageKeys [][]byte) ([]byte, error) {
	cometBFTClient, err := createCometBFTClient(cometBFTRPCAddress)
	if err != nil {
		return nil, err
	}

	defer cometBFTClient.Stop()

	accountResult, err := getProof(cometBFTClient, serializedHashedAccountAddress, storageKeys, blockNumOrHash)
	if err != nil {
		return nil, err
	}

	// fmt.Printf("ACCOUNT RESULT: %+v\n", accountResult)

	return json.Marshal(accountResult)
}
