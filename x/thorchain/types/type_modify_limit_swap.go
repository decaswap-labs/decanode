package types

import (
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

type ModifyLimitSwaps []ModifyLimitSwap

func NewModifyLimitSwap(from common.Address, source, target common.Coin, mod cosmos.Uint) ModifyLimitSwap {
	return ModifyLimitSwap{
		From:                 from,
		Source:               source,
		Target:               target,
		ModifiedTargetAmount: mod,
	}
}
