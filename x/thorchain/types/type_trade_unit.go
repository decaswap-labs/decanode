package types

import (
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

func NewTradeUnit(asset common.Asset) TradeUnit {
	return TradeUnit{
		Asset: asset,
		Units: cosmos.ZeroUint(),
		Depth: cosmos.ZeroUint(),
	}
}

func (tu TradeUnit) Key() string {
	return tu.Asset.String()
}
