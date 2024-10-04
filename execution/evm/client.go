package evm

const (
	EVM = "evm"
)

type EVMClient struct {
}

func (client *EVMClient) Name() string {
	return EVM
}
