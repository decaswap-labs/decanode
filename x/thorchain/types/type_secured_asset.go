package types

import (
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

func NewSecuredAsset(asset common.Asset) SecuredAsset {
	return SecuredAsset{
		Asset: asset,
		Depth: cosmos.ZeroUint(),
	}
}

func (tu SecuredAsset) Key() string {
	return tu.Asset.String()
}
