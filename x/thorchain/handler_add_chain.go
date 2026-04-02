package thorchain

import (
	"fmt"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/keeper"
)

const AddChainSupermajority = 21

type AddChainHandler struct {
	mgr Manager
}

func NewAddChainHandler(mgr Manager) AddChainHandler {
	return AddChainHandler{
		mgr: mgr,
	}
}

func (h AddChainHandler) HandleAddChain(ctx cosmos.Context, chain common.Chain, signers []cosmos.AccAddress) (*cosmos.Result, error) {
	if chain.IsEmpty() {
		return nil, fmt.Errorf("chain identifier cannot be empty")
	}

	if !h.hasSupermajority(ctx, h.mgr.Keeper(), signers) {
		return nil, fmt.Errorf("add chain requires %d/%d node signatures", AddChainSupermajority, 30)
	}

	gasAsset := chain.GetGasAsset()
	pool, err := h.mgr.Keeper().GetPool(ctx, gasAsset)
	if err != nil {
		return nil, fmt.Errorf("fail to get pool: %w", err)
	}

	if !pool.IsEmpty() {
		return nil, fmt.Errorf("pool for chain %s already exists", chain)
	}

	pool = NewPool()
	pool.Asset = gasAsset
	pool.Status = PoolStaged
	pool.StatusSince = ctx.BlockHeight()
	pool.BalanceDeca = cosmos.ZeroUint()
	pool.BalanceAsset = cosmos.ZeroUint()

	err = h.mgr.Keeper().SetPool(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("fail to set pool: %w", err)
	}

	ctx.Logger().Info("pool bootstrap pending first deposit",
		"chain", chain,
		"asset", gasAsset,
	)

	return &cosmos.Result{}, nil
}

func (h AddChainHandler) hasSupermajority(ctx cosmos.Context, k keeper.Keeper, signers []cosmos.AccAddress) bool {
	active, err := k.ListActiveValidators(ctx)
	if err != nil {
		ctx.Logger().Error("fail to list active validators", "error", err)
		return false
	}

	activeMap := make(map[string]bool)
	for _, na := range active {
		activeMap[na.NodeAddress.String()] = true
	}

	validSigners := 0
	seen := make(map[string]bool)
	for _, signer := range signers {
		addr := signer.String()
		if activeMap[addr] && !seen[addr] {
			validSigners++
			seen[addr] = true
		}
	}

	return validSigners >= AddChainSupermajority
}
