package thorchain

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/common/tokenlist"
	"github.com/decaswap-labs/decanode/x/thorchain/keeper"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

// MsgHandler is an interface expect all handler to implement
type MsgHandler interface {
	Run(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error)
}

// SwapHandlerWithEmit is a specialized interface for handlers that need to return emit values
type SwapHandlerWithEmit interface {
	RunWithEmit(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, cosmos.Uint, error)
}

// NewInternalHandler returns a handler for "thorchain" internal type messages.
func NewInternalHandler(mgr Manager) cosmos.Handler {
	return func(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error) {
		handlerMap := getInternalHandlerMapping(mgr)
		h, ok := handlerMap[sdk.MsgTypeURL(msg)]
		if !ok {
			errMsg := fmt.Sprintf("Unrecognized thorchain Msg type: %v", sdk.MsgTypeURL(msg))
			return nil, cosmos.ErrUnknownRequest(errMsg)
		}

		// CacheContext() returns a context which caches all changes and only forwards
		// to the underlying context when commit() is called. Call commit() only when
		// the handler succeeds, otherwise return error and the changes will be discarded.
		// On commit, cached events also have to be explicitly emitted.
		cacheCtx, commit := ctx.CacheContext()
		res, err := h.Run(cacheCtx, msg)
		if err == nil {
			// Success, commit the cached changes and events
			commit()
		}

		return res, err
	}
}

func getInternalHandlerMapping(mgr Manager) map[string]MsgHandler {
	// New arch handlers
	m := make(map[string]MsgHandler)
	m[sdk.MsgTypeURL(&MsgOutboundTx{})] = NewOutboundTxHandler(mgr)
	m[sdk.MsgTypeURL(&MsgSwap{})] = NewSwapHandler(mgr)
	m[sdk.MsgTypeURL(&MsgReserveContributor{})] = NewReserveContributorHandler(mgr)
	m[sdk.MsgTypeURL(&MsgBond{})] = NewBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgUnBond{})] = NewUnBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgReBond{})] = NewReBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgLeave{})] = NewLeaveHandler(mgr)
	m[sdk.MsgTypeURL(&MsgMaint{})] = NewMaintHandler(mgr)
	m[sdk.MsgTypeURL(&MsgDonate{})] = NewDonateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgRefundTx{})] = NewRefundHandler(mgr)
	m[sdk.MsgTypeURL(&MsgMigrate{})] = NewMigrateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgRagnarok{})] = NewRagnarokHandler(mgr)
	m[sdk.MsgTypeURL(&MsgNoOp{})] = NewNoOpHandler(mgr)
	m[sdk.MsgTypeURL(&MsgConsolidate{})] = NewConsolidateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgModifyLimitSwap{})] = NewModifyLimitSwapHandler(mgr)
	m[sdk.MsgTypeURL(&MsgOperatorRotate{})] = NewOperatorRotateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgSwapRequest{})] = NewSwapRequestHandler(mgr)
	return m
}

func getMsgSwapFromMemo(ctx cosmos.Context, keeper keeper.Keeper, memo SwapMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	if memo.Destination.IsEmpty() {
		memo.Destination = tx.Tx.FromAddress
	}

	// Determine version based on configuration
	version := types.SwapVersion_v1
	if keeper.AdvSwapQueueEnabled(ctx) {
		version = types.SwapVersion_v2
	}

	return NewMsgSwap(tx.Tx, memo.GetAsset(), memo.Destination, memo.SlipLimit, memo.AffiliateAddress, memo.AffiliateBasisPoints, memo.GetDexAggregator(), memo.GetDexTargetAddress(), memo.GetDexTargetLimit(), memo.GetSwapType(), memo.GetStreamQuantity(), memo.GetStreamInterval(), version, signer), nil
}

func getMsgDonateFromMemo(memo DonateMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	runeCoin := tx.Tx.Coins.GetCoin(common.DecaAsset())
	assetCoin := tx.Tx.Coins.GetCoin(memo.GetAsset())
	return NewMsgDonate(tx.Tx, memo.GetAsset(), runeCoin.Amount, assetCoin.Amount, signer), nil
}

func getMsgModifyLimitSwap(memo ModifyLimitSwapMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	// Get the deposit asset and amount from the transaction
	var depositAsset common.Asset
	var depositAmount cosmos.Uint
	if len(tx.Tx.Coins) > 0 {
		depositAsset = tx.Tx.Coins[0].Asset
		depositAmount = tx.Tx.Coins[0].Amount
	}
	return NewMsgModifyLimitSwap(tx.Tx.FromAddress, memo.Source, memo.Target, memo.ModifiedTargetAmount, signer, depositAsset, depositAmount), nil
}

func getMsgRefundFromMemo(memo RefundMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgRefundTx(tx, memo.GetTxID(), signer), nil
}

func getMsgOutboundFromMemo(memo OutboundMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgOutboundTx(tx, memo.GetTxID(), signer), nil
}

func getMsgMigrateFromMemo(memo MigrateMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgMigrate(tx, memo.GetBlockHeight(), signer), nil
}

func getMsgRagnarokFromMemo(memo RagnarokMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgRagnarok(tx, memo.GetBlockHeight(), signer), nil
}

func getMsgLeaveFromMemo(memo LeaveMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgLeave(tx.Tx, memo.GetAccAddress(), signer), nil
}

func getMsgBondFromMemo(memo BondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	coin := tx.Tx.Coins.GetCoin(common.DecaAsset())
	return NewMsgBond(tx.Tx, memo.GetAccAddress(), coin.Amount, tx.Tx.FromAddress, memo.BondProviderAddress, signer, memo.NodeOperatorFee), nil
}

func getMsgUnbondFromMemo(memo UnbondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgUnBond(tx.Tx, memo.GetAccAddress(), memo.GetAmount(), tx.Tx.FromAddress, memo.BondProviderAddress, signer), nil
}

func getMsgRebondFromMemo(memo RebondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgReBond(tx.Tx, memo.GetNodeAddress(), memo.GetNewProviderAddress(), memo.GetAmount(), signer), nil
}

func getMsgMaintFromMemo(memo MaintMemo, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return types.NewMsgMaint(memo.GetAccAddress(), signer), nil
}

func processOneTxIn(ctx cosmos.Context, keeper keeper.Keeper, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	if len(tx.Tx.Coins) != 1 {
		return nil, cosmos.ErrInvalidCoins("only send 1 coins per message")
	}

	memo, err := ParseMemoWithTHORNames(ctx, keeper, tx.Tx.Memo)
	if err != nil {
		ctx.Logger().Error("fail to parse memo", "error", err)
		return nil, err
	}

	// THORNode should not have one tx across chain, if it is cross chain it should be separate tx
	var newMsg cosmos.Msg
	// interpret the memo and initialize a corresponding msg event
	switch m := memo.(type) {
	case AddLiquidityMemo:
		return nil, fmt.Errorf("add liquidity is not supported")
	case WithdrawLiquidityMemo:
		return nil, fmt.Errorf("withdraw liquidity is not supported")
	case SwapMemo:
		m.Asset = fuzzyAssetMatch(ctx, keeper, m.Asset)
		m.DexTargetAddress = externalAssetMatch(m.Asset.GetChain(), m.DexTargetAddress)
		newMsg, err = getMsgSwapFromMemo(ctx, keeper, m, tx, signer)
	case ModifyLimitSwapMemo:
		newMsg, err = getMsgModifyLimitSwap(m, tx, signer)
	case DonateMemo:
		m.Asset = fuzzyAssetMatch(ctx, keeper, m.Asset)
		newMsg, err = getMsgDonateFromMemo(m, tx, signer)
	case RefundMemo:
		newMsg, err = getMsgRefundFromMemo(m, tx, signer)
	case OutboundMemo:
		newMsg, err = getMsgOutboundFromMemo(m, tx, signer)
	case MigrateMemo:
		newMsg, err = getMsgMigrateFromMemo(m, tx, signer)
	case BondMemo:
		newMsg, err = getMsgBondFromMemo(m, tx, signer)
	case UnbondMemo:
		newMsg, err = getMsgUnbondFromMemo(m, tx, signer)
	case RebondMemo:
		newMsg, err = getMsgRebondFromMemo(m, tx, signer)
	case RagnarokMemo:
		newMsg, err = getMsgRagnarokFromMemo(m, tx, signer)
	case LeaveMemo:
		newMsg, err = getMsgLeaveFromMemo(m, tx, signer)
	case ReserveMemo:
		res := NewReserveContributor(tx.Tx.FromAddress, tx.Tx.Coins.GetCoin(common.DecaAsset()).Amount)
		newMsg = NewMsgReserveContributor(tx.Tx, res, signer)
	case NoOpMemo:
		newMsg = NewMsgNoOp(tx, signer, m.Action)
	case ConsolidateMemo:
		newMsg = NewMsgConsolidate(tx, signer)
	case MaintMemo:
		newMsg, err = getMsgMaintFromMemo(m, signer)
	case OperatorRotateMemo:
		newMsg = NewMsgOperatorRotate(signer, m.OperatorAddress, tx.Tx.Coins[0])
	default:
		return nil, errInvalidMemo
	}

	if err != nil {
		return newMsg, err
	}

	newMsgV, ok := newMsg.(sdk.HasValidateBasic)
	if !ok {
		return newMsg, fmt.Errorf("msg does not implement sdk.HasValidateBasic: %T", newMsg)
	}

	return newMsg, newMsgV.ValidateBasic()
}

func fuzzyAssetMatch(ctx cosmos.Context, keeper keeper.Keeper, origAsset common.Asset) common.Asset {
	asset := origAsset.GetLayer1Asset()
	// if it's already an exact match with successfully-added liquidity, return it immediately
	pool, err := keeper.GetPool(ctx, asset)
	if err != nil {
		return origAsset
	}
	// Only check BalanceDeca after checking the error so that no panic if there were an error.
	if !pool.BalanceDeca.IsZero() {
		return origAsset
	}

	parts := strings.Split(asset.Symbol.String(), "-")
	hasNoSymbol := len(parts) < 2 || len(parts[1]) == 0
	var symbol string
	if !hasNoSymbol {
		symbol = strings.ToLower(parts[1])
	}
	winner := NewPool()
	// if no asset found, return original asset
	winner.Asset = origAsset
	iterator := keeper.GetPoolIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		if err = keeper.Cdc().Unmarshal(iterator.Value(), &pool); err != nil {
			ctx.Logger().Error("fail to fetch pool", "asset", asset, "err", err)
			continue
		}

		// check chain match
		if !asset.Chain.Equals(pool.Asset.Chain) {
			continue
		}

		// check ticker match
		if !asset.Ticker.Equals(pool.Asset.Ticker) {
			continue
		}

		// check if no symbol given (ie "USDT" or "USDT-")
		if hasNoSymbol {
			// Use LTE rather than LT so this function can only return origAsset or a match
			if winner.BalanceDeca.LTE(pool.BalanceDeca) {
				winner = pool
			}
			continue
		}

		if strings.HasSuffix(strings.ToLower(pool.Asset.Symbol.String()), symbol) {
			// Use LTE rather than LT so this function can only return origAsset or a match
			if winner.BalanceDeca.LTE(pool.BalanceDeca) {
				winner = pool
			}
			continue
		}
	}
	// Since the Chain and Ticker must already match, replace just the Symbol with the winner's,
	// keeping other fields like Synth and Trade the same as the original.
	origAsset.Symbol = winner.Asset.Symbol
	return origAsset
}

func externalAssetMatch(chain common.Chain, hint string) string {
	if len(hint) == 0 {
		return hint
	}
	if chain.IsEVM() {
		// find all potential matches
		firstMatch := ""
		addrHint := strings.ToLower(hint)
		for _, token := range tokenlist.GetEVMTokenList(chain).Tokens {
			if strings.HasSuffix(strings.ToLower(token.Address), addrHint) {
				// store first found address
				if firstMatch == "" {
					firstMatch = token.Address
				} else {
					return hint
				}
			}
		}
		if firstMatch != "" {
			return firstMatch
		}
	}
	return hint
}
