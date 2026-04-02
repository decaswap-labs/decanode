//go:build !regtest
// +build !regtest

package thorchain

import (
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/keeper"
)

func InitGenesis(ctx cosmos.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	return initGenesis(ctx, keeper, data)
}
