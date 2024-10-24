package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

func (c *EVMClient) ExchangeCapabilities(cap []string) ([]string, error) {
	result := make([]string, 0)
	if err := c.engineClient.Client().CallContext(
		c.ctx, &result, ExchangeCapabilities, &cap,
	); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *EVMClient) ForkchoiceUpdated(state *ForkchoiceState, attributes any) (*ForkchoiceResponse, error) {
	result := &ForkchoiceResponse{}

	if err := c.engineClient.Client().CallContext(
		c.ctx, &result, ForkchoiceUpdatedMethodV3, state, attributes,
	); err != nil {
		return nil, err
	}

	if (result.PayloadStatus == PayloadStatus{}) {
		return nil, fmt.Errorf("nil response from forkchoiceupdate")
	}

	return result, nil
}

func (c *EVMClient) GetPayload(payloadID PayloadID) (*ExecutionPayloadEnvelope, error) {
	result := &ExecutionPayloadEnvelope{}

	err := c.engineClient.Client().CallContext(
		c.ctx, &result, GetPayloadMethodV3, payloadID,
	)

	return result, err
}

func (c *EVMClient) NewPayload(payload ExecutionPayloadEnvelope, parentBlockRoot *common.Hash) (*PayloadStatus, error) {
	result := &PayloadStatus{}
	if err := c.engineClient.Client().CallContext(
		c.ctx, result, NewPayloadMethodV3, payload, parentBlockRoot,
	); err != nil {
		return nil, err
	}
	return result, nil
}
