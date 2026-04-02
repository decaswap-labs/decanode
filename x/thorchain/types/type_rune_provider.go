package types

import (
	"github.com/decaswap-labs/decanode/common/cosmos"
)

func NewRUNEProvider(addr cosmos.AccAddress) RUNEProvider {
	return RUNEProvider{
		DecaAddress: addr,
		Units:       cosmos.ZeroUint(),
	}
}

func (rp RUNEProvider) Key() string {
	return rp.DecaAddress.String()
}
