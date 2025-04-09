package app

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

func (app *ScalerizeApp) StartStateRouter(clientType string) {
	os.Remove(socketPath)

	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	app.Logger().Info("Listening on: ", socketPath)

	for {
		fmt.Println("CONNECTING TO UNIX SOCKET SERVER FOR DB")
		conn, err := l.Accept()
		if err != nil {
			app.Logger().Error("Error accepting connection to Scalerize DB Router: ", err)
			continue
		}

		app.Logger().Info("New client connected to Scalerize Database Router")

		fmt.Println("New client connected to Scalerize Database Router")

		go app.handleStateQuery(conn)
	}
}

func (app *ScalerizeApp) handleStateQuery(conn net.Conn) {
	defer conn.Close()

	fmt.Println("STARTING HANDLING CONNECTION")

	for {
		var (
			response []byte
		)

		// 1st byte contains the operation
		// next 8 bytes contains block number
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
		fmt.Println("OPERATION: ", operation)

		// height := data[1:9]
		// fmt.Println("HEIGHT: ", height)

		switch operation {
		case OP_STATE_ROOT:
			if len(data) != 1+EthBlockNumberBytes {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			height := data[1 : 1+EthBlockNumberBytes]
			fmt.Println("HEIGHT: ", height)

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

			if len(data) < 2 {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			if data[1] == 0 {
				if len(data) < 2+EthBlockNumberBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				bn := data[2 : 2+EthBlockNumberBytes]
				bnInt := int64(binary.BigEndian.Uint64(bn))
				blockNumOrHash.BlockNumber = &bnInt
				blockSpecBytes = EthBlockNumberBytes
			} else if data[1] == 1 {
				if len(data) < 2+EthBlockHashBytes {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

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
			if len(data) < addressEnd {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
				break
			}

			address := data[addressStart:addressEnd]

			if len(data) == 2+blockSpecBytes+SerializedHashedAccountsKeyBytes {
				storageKeys = [][]byte{}
			} else {
				storageKeysBytes := len(data) - 2 - blockSpecBytes - SerializedHashedAccountsKeyBytes
				if storageKeysBytes%(SerializedHashedStoragesKeyBytes+SerializedHashedStoragesSubKeyBytes) != 0 {
					response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
					break
				}

				noOfStorageKeys := storageKeysBytes / (SerializedHashedStoragesKeyBytes + SerializedHashedStoragesSubKeyBytes)

				for i := range noOfStorageKeys {
					storageKey := data[1+EthBlockNumberBytes+SerializedHashedAccountsKeyBytes+i*(SerializedHashedStoragesKeyBytes+SerializedHashedStoragesSubKeyBytes) : (i+1)*(SerializedHashedStoragesKeyBytes+SerializedHashedStoragesSubKeyBytes)]
					storageKeys = append(storageKeys, storageKey)
				}
			}

			resp, err := app.StateProof(&blockNumOrHash, address, storageKeys)
			if err != nil {
				response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidRequestData.Error())...)
			} else {
				response = append([]byte{STATUS_SUCCESS}, resp...)
			}
		default:
			response = append([]byte{STATUS_ERROR}, []byte(ErrInvalidOperationCode.Error())...)
		}

		app.writeToConn(conn, response)
	}
}

func (app *ScalerizeApp) StateRoot(height int64) ([]byte, error) {
	if height < -1 {
		return nil, ErrInvalidRequestData
	}

	if height == -1 {
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

	return json.Marshal(accountResult)
}
