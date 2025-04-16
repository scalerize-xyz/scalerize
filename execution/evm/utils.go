package evm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

func (evmClient *EVMClient) getSyncStatus() (bool, error) {
	status, err := evmClient.cosmosRPCClient.Status(context.Background())
	if err != nil {
		return false, err
	}

	return status.SyncInfo.CatchingUp, nil
}

func generateRandao() (common.Hash, error) {
	var hash common.Hash
	_, err := rand.Read(hash[:])
	return hash, err
}

func hexToAddress(hexStr string) (common.Address, error) {
	var addr [20]byte

	decoded, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return addr, err
	}

	if len(decoded) != 20 {
		return addr, fmt.Errorf("invalid length: got %d, want 20", len(decoded))
	}

	copy(addr[:], decoded)
	return addr, nil
}
