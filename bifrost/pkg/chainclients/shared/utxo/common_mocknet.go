//go:build mocknet
// +build mocknet

package utxo

import (
	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

func GetConfMulBasisPoint(chain string, bridge thorclient.ThorchainBridge) (cosmos.Uint, error) {
	return cosmos.NewUint(1), nil
}

func MaxConfAdjustment(confirm uint64, chain string, bridge thorclient.ThorchainBridge) (uint64, error) {
	return 1, nil
}
