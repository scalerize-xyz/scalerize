package evm

const (
	// NewPayloadMethodV3 for creating a new payload in Deneb.
	NewPayloadMethodV3 = "engine_newPayloadV3"
	// ForkchoiceUpdatedMethodV3 for updating fork choice in Deneb.
	ForkchoiceUpdatedMethodV3 = "engine_forkchoiceUpdatedV3"
	// GetPayloadMethodV3 for retrieving a payload in Deneb.
	GetPayloadMethodV3 = "engine_getPayloadV3"
	// BlockByHashMethod for retrieving a block by its hash.
	BlockByHashMethod = "eth_getBlockByHash"
	// BlockByNumberMethod for retrieving a block by its number.
	BlockByNumberMethod = "eth_getBlockByNumber"
	// BlockNumberMethod for retrieving latest block number.
	BlockNumberMethod = "eth_blockNumber"
	// ExchangeCapabilities for exchanging capabilities with the peer.
	ExchangeCapabilities = "engine_exchangeCapabilities"
	// GetClientVersionV1 for retrieving the capabilities of the peer.
	GetClientVersionV1 = "engine_getClientVersionV1"
)

func ScalerizeSupportedCapabilities() []string {
	return []string{
		ForkchoiceUpdatedMethodV3,
		GetPayloadMethodV3,
		NewPayloadMethodV3,
		GetClientVersionV1,
	}
}
