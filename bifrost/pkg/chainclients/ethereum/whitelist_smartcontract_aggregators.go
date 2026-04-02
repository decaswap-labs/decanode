package ethereum

import (
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/x/thorchain/aggregators"
)

func LatestAggregatorContracts() []common.Address {
	addrs := []common.Address{}
	for _, agg := range aggregators.DexAggregators() {
		if agg.Chain.Equals(common.ETHChain) {
			addrs = append(addrs, common.Address(agg.Address))
		}
	}
	return addrs
}
