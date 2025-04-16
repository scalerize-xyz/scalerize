package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ABCIHandler interface {
	PrepareProposal() sdk.PrepareProposalHandler
	ProcessProposal() sdk.ProcessProposalHandler
	PreBlock() sdk.PreBlocker
	EndBlock() sdk.EndBlocker
}
