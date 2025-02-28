package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	// Connect to Ethereum client
	client, err := ethclient.Dial("http://127.0.0.1:8545")
	if err != nil {
		log.Fatal("Connection error:", err)
	}
	defer client.Close()

	// Load private key
	privateKey, err := crypto.HexToECDSA("3c9ccc6a117204e9dca913ffc04b2ce235cafba8feb6dac15c47594c315d525d")
	if err != nil {
		log.Fatal("Private key error:", err)
	}

	// Derive public address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Public key conversion error")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	// Get chain ID
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatal("Network ID error:", err)
	}

	// Get account nonce
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatal("Nonce error:", err)
	}

	// Recipient address
	toAddress := common.HexToAddress("0x4592d8f8d7b001e72cb26a73e4fa1806a51ac79d")
	value := big.NewInt(1000000000000000000) // 1 ETH

	// Get current base fee and priority fee
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal("Header error:", err)
	}
	baseFee := header.BaseFee

	tipCap, err := client.SuggestGasTipCap(context.Background())
	if err != nil {
		log.Fatal("Gas tip suggestion error:", err)
	}

	// Add 50% to suggested tip for priority
	tipCap = new(big.Int).Mul(tipCap, big.NewInt(100))
	tipCap.Div(tipCap, big.NewInt(100))

	// Calculate max fee (base fee * 2 + priority fee)
	maxFee := new(big.Int).Add(
		new(big.Int).Mul(baseFee, big.NewInt(2)),
		tipCap,
	)

	// Set gas limit with 10% buffer
	gasLimit := uint64(21000)
	gasLimit = uint64(float64(gasLimit) * 5)

	// Check account balance
	balance, err := client.BalanceAt(context.Background(), fromAddress, nil)
	if err != nil {
		log.Fatal("Balance check error:", err)
	}

	cost := new(big.Int).Mul(maxFee, big.NewInt(int64(gasLimit)))
	cost.Add(cost, value)

	if balance.Cmp(cost) < 0 {
		log.Fatal("Insufficient funds. Required:", cost, "Available:", balance)
	}

	// Create EIP-1559 transaction
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		To:        &toAddress,
		Value:     value,
		Gas:       gasLimit,
		GasFeeCap: maxFee,
		GasTipCap: tipCap,
	}

	tx := types.NewTx(txData)

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), privateKey)
	if err != nil {
		log.Fatal("Signing error:", err)
	}

	// Send transaction with retry logic
	maxRetries := 3
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		err = client.SendTransaction(context.Background(), signedTx)
		if err == nil {
			fmt.Printf("Transaction successfully sent! Hash: %s\n", signedTx.Hash().Hex())
			fmt.Printf("Max Fee: %s Wei\n", maxFee.String())
			fmt.Printf("Priority Fee: %s Wei\n", tipCap.String())
			return
		}

		fmt.Printf("Attempt %d failed: %v\n", i+1, err)

		// Increase tip by 15% for next attempt
		tipCap = new(big.Int).Mul(tipCap, big.NewInt(115))
		tipCap.Div(tipCap, big.NewInt(100))

		// Update transaction with new fees
		txData.GasTipCap = tipCap
		txData.GasFeeCap = new(big.Int).Add(baseFee, tipCap)
		tx = types.NewTx(txData)

		signedTx, err = types.SignTx(tx, types.LatestSignerForChainID(chainID), privateKey)
		if err != nil {
			log.Fatal("Resigning error:", err)
		}

		time.Sleep(retryDelay)
	}

	log.Fatal("Failed to send transaction after", maxRetries, "attempts")
}
