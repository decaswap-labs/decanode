package thorchain

import (
	"fmt"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

type SwapRequestHandler struct {
	mgr Manager
}

func NewSwapRequestHandler(mgr Manager) SwapRequestHandler {
	return SwapRequestHandler{
		mgr: mgr,
	}
}

func (h SwapRequestHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*types.MsgSwapRequest)
	if !ok {
		return nil, errInvalidMessage
	}
	err := h.validate(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("MsgSwapRequest failed validation", "error", err)
		return nil, err
	}
	return h.handle(ctx, *msg)
}

func (h SwapRequestHandler) validate(ctx cosmos.Context, msg types.MsgSwapRequest) error {
	err := msg.ValidateBasic()
	if err != nil {
		return err
	}

	if h.mgr.Keeper().IsGlobalTradingHalted(ctx) {
		return fmt.Errorf("trading is halted")
	}

	if msg.SourceAsset.IsEmpty() {
		return fmt.Errorf("source asset cannot be empty")
	}
	if msg.TargetAsset.IsEmpty() {
		return fmt.Errorf("target asset cannot be empty")
	}
	if msg.SourceAsset.Equals(msg.TargetAsset) {
		return fmt.Errorf("source and target assets cannot be the same")
	}

	if msg.Destination.IsEmpty() {
		return fmt.Errorf("destination address cannot be empty")
	}

	if msg.Amount.IsZero() {
		return fmt.Errorf("amount cannot be zero")
	}

	if !msg.TargetAsset.Chain.IsEmpty() {
		_, err = common.NewAddress(msg.Destination.String())
		if err != nil {
			return fmt.Errorf("invalid destination address for chain %s: %w", msg.TargetAsset.Chain, err)
		}
	}

	return nil
}

func (h SwapRequestHandler) handle(ctx cosmos.Context, msg types.MsgSwapRequest) (*cosmos.Result, error) {
	version := types.SwapVersion_v1
	if h.mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		version = types.SwapVersion_v2
	}

	fromAddr, err := common.NewAddress(msg.Signer.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create from address: %w", err)
	}

	tx := common.NewTx(
		common.BlankTxID,
		fromAddr,
		msg.Destination,
		common.Coins{common.NewCoin(msg.SourceAsset, msg.Amount)},
		common.Gas{},
		"swap-request",
	)

	swapType := types.SwapType_market
	quantity := msg.StreamingQuantity
	interval := msg.StreamingInterval

	swapMsg := NewMsgSwap(
		tx,
		msg.TargetAsset,
		msg.Destination,
		cosmos.ZeroUint(),
		common.NoAddress,
		cosmos.ZeroUint(),
		"", "", nil,
		swapType,
		quantity,
		interval,
		version,
		msg.Signer,
	)

	err = h.mgr.Keeper().SetSwapQueueItem(ctx, *swapMsg, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to queue swap request: %w", err)
	}

	ctx.Logger().Info("swap request queued",
		"source", msg.SourceAsset.String(),
		"target", msg.TargetAsset.String(),
		"amount", msg.Amount.String(),
		"destination", msg.Destination.String(),
	)

	return &cosmos.Result{}, nil
}
