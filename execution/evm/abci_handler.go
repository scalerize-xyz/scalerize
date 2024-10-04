package evm

type ABCIHandler struct {
	client EVMClient
}

func NewEVMABCIHandler() *ABCIHandler {
	return &ABCIHandler{
		// client: client,
	}
}
