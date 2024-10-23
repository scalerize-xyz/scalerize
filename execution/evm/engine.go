package evm

import (
	"context"
)

func (c *EVMClient) ExchangeCapabilities(ctx context.Context, cap []string) ([]string, error) {
	result := make([]string, 0)
	if err := c.engineClient.Client().CallContext(
		ctx, &result, ExchangeCapabilities, &cap,
	); err != nil {
		return nil, err
	}

	return result, nil
}
