package abci

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ABCIHandler interface {
	PrepareProposal() sdk.PrepareProposalHandler
	ProcessProposal() sdk.ProcessProposalHandler
	PreBlock() sdk.PreBlocker
	EndBlock() sdk.EndBlocker
	// ExtendVote() sdk.ExtendVoteHandler
	// VerifyVoteExtension() sdk.VerifyVoteExtensionHandler
	// BeginBlocker() sdk.BeginBlocker
	// EndBlocker() sdk.EndBlocker
}

type ScalerizeABCIHandler struct {
	// ABCIHandler
}

func NewScalerizeABCIHandler(h ABCIHandler) *ScalerizeABCIHandler {
	return &ScalerizeABCIHandler{
		// ABCIHandler: h,
	}
}
