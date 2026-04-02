package suites

import (
	"fmt"
	rand "math/rand/v2"

	acommon "github.com/decaswap-labs/decanode/test/simulation/actors/common"
	"github.com/decaswap-labs/decanode/test/simulation/actors/core"
	"github.com/decaswap-labs/decanode/test/simulation/pkg/thornode"
	. "github.com/decaswap-labs/decanode/test/simulation/pkg/types"
)

////////////////////////////////////////////////////////////////////////////////////////
// Ragnarok
////////////////////////////////////////////////////////////////////////////////////////

func Ragnarok(rng *rand.Rand) *Actor {
	a := NewActor("Ragnarok", rng)

	// ragnarok all gas asset pools (should apply to tokens implicitly)
	for _, chain := range acommon.SimChains {
		a.Children[core.NewRagnarokPoolActor(chain.GetGasAsset(), rng)] = true
	}

	// verify pool removals
	verify := NewActor("Ragnarok-Verify", rng)
	verify.Ops = append(verify.Ops, func(config *OpConfig) OpResult {
		pools, err := thornode.GetPools()
		if err != nil {
			return OpResult{Finish: true, Error: err}
		}

		// no chains should have pools
		if len(pools) != 0 {
			return OpResult{
				Finish: true,
				Error:  fmt.Errorf("found %d pools after ragnarok", len(pools)),
			}
		}

		return OpResult{Finish: true}
	})
	a.Append(verify)

	return a
}
