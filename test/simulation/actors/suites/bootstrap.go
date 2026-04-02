package suites

import (
	"fmt"
	rand "math/rand/v2"

	"github.com/rs/zerolog/log"
	"github.com/decaswap-labs/decanode/common"
	acommon "github.com/decaswap-labs/decanode/test/simulation/actors/common"
	"github.com/decaswap-labs/decanode/test/simulation/actors/core"
	"github.com/decaswap-labs/decanode/test/simulation/pkg/evm"
	"github.com/decaswap-labs/decanode/test/simulation/pkg/thornode"
	. "github.com/decaswap-labs/decanode/test/simulation/pkg/types"
)

////////////////////////////////////////////////////////////////////////////////////////
// Bootstrap
////////////////////////////////////////////////////////////////////////////////////////

func Bootstrap(rng *rand.Rand) *Actor {
	a := NewActor("Bootstrap", rng)

	pools, err := thornode.GetPools()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get pools")
	}

	// bootstrap pools for all chains
	count := 0
	for _, chain := range acommon.SimChains {
		count++
		a.Children[core.NewDualLPActor(chain.GetGasAsset(), rng)] = true
	}

	// create token pools
	tokenPools := NewActor("Bootstrap-TokenPools", rng)
	for _, chain := range acommon.SimChains {
		if !chain.IsEVM() {
			continue
		}
		// BSC not compatible with sim tests
		if chain.Equals(common.BSCChain) {
			continue
		}
		count++

		for asset := range evm.Tokens(chain) {
			tokenPools.Children[core.NewDualLPActor(asset, rng)] = true
		}
	}
	a.Append(tokenPools)

	// verify pools
	verify := NewActor("Bootstrap-Verify", rng)
	verify.Ops = append(verify.Ops, func(config *OpConfig) OpResult {
		pools, err = thornode.GetPools()
		if err != nil {
			return OpResult{Finish: true, Error: err}
		}

		// all pools should be available
		for _, pool := range pools {
			if pool.Status != "Available" {
				return OpResult{
					Finish: true,
					Error:  fmt.Errorf("pool %s not available", pool.Asset),
				}
			}
		}

		// all chains should have pools
		if len(pools) != count {
			return OpResult{
				Finish: true,
				Error:  fmt.Errorf("expected %d pools, got %d", count, len(pools)),
			}
		}

		return OpResult{Finish: true}
	},
	)
	a.Append(verify)

	return a
}
