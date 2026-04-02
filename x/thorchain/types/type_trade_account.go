package types

import (
	"fmt"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

func NewTradeAccount(addr cosmos.AccAddress, asset common.Asset) TradeAccount {
	return TradeAccount{
		Owner: addr,
		Asset: asset,
		Units: cosmos.ZeroUint(),
	}
}

func (tr TradeAccount) Key() string {
	return fmt.Sprintf("%s/%s", tr.Owner.String(), tr.Asset.String())
}
