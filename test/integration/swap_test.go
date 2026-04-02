package integration

import (
	"testing"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

func TestIntegrationSwap_QueueSwapRequest(t *testing.T) {
	env := setupIntegrationEnv(t)

	setupPoolForTest(t, env.Ctx, env.Keeper, common.BTCAsset)
	setupPoolForTest(t, env.Ctx, env.Keeper, common.ZECAsset)

	signer := types.GetRandomBech32Addr()
	fundAccount(t, env.Ctx, env.Keeper, signer, 100*common.One)

	destination := types.GetRandomBTCAddress()
	fromAddr, err := common.NewAddress(signer.String())
	if err != nil {
		t.Fatalf("failed to create from address: %v", err)
	}

	tx := common.NewTx(
		common.BlankTxID,
		fromAddr,
		common.Address(destination),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1*common.One))},
		common.Gas{},
		"swap-request",
	)

	swapMsg := types.NewMsgSwap(
		tx,
		common.ZECAsset,
		common.Address(destination),
		cosmos.ZeroUint(),
		common.NoAddress,
		cosmos.ZeroUint(),
		"", "", nil,
		types.SwapType_market,
		0, 0,
		types.SwapVersion_v1,
		signer,
	)

	err = env.Keeper.SetSwapQueueItem(env.Ctx, *swapMsg, 0)
	if err != nil {
		t.Fatalf("failed to queue swap: %v", err)
	}

	iter := env.Keeper.GetSwapQueueIterator(env.Ctx)
	defer iter.Close()

	count := 0
	for ; iter.Valid(); iter.Next() {
		count++
	}
	if count != 1 {
		t.Fatalf("expected 1 swap in queue, got %d", count)
	}
}

func TestIntegrationSwap_StreamingSwapChunks(t *testing.T) {
	env := setupIntegrationEnv(t)

	setupPoolForTest(t, env.Ctx, env.Keeper, common.BTCAsset)
	setupPoolForTest(t, env.Ctx, env.Keeper, common.ZECAsset)

	signer := types.GetRandomBech32Addr()
	fundAccount(t, env.Ctx, env.Keeper, signer, 100*common.One)

	destination := types.GetRandomBTCAddress()
	fromAddr, err := common.NewAddress(signer.String())
	if err != nil {
		t.Fatalf("failed to create from address: %v", err)
	}

	streamingQuantity := uint64(5)
	streamingInterval := uint64(1)

	tx := common.NewTx(
		common.BlankTxID,
		fromAddr,
		common.Address(destination),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(10*common.One))},
		common.Gas{},
		"swap-request-streaming",
	)

	swapMsg := types.NewMsgSwap(
		tx,
		common.ZECAsset,
		common.Address(destination),
		cosmos.ZeroUint(),
		common.NoAddress,
		cosmos.ZeroUint(),
		"", "", nil,
		types.SwapType_market,
		streamingQuantity,
		streamingInterval,
		types.SwapVersion_v1,
		signer,
	)

	err = env.Keeper.SetSwapQueueItem(env.Ctx, *swapMsg, 0)
	if err != nil {
		t.Fatalf("failed to queue streaming swap: %v", err)
	}

	iter := env.Keeper.GetSwapQueueIterator(env.Ctx)
	defer iter.Close()

	found := false
	for ; iter.Valid(); iter.Next() {
		var msg types.MsgSwap
		unmarshalErr := env.Keeper.Cdc().Unmarshal(iter.Value(), &msg)
		if unmarshalErr != nil {
			t.Fatalf("failed to unmarshal swap msg: %v", unmarshalErr)
		}
		if msg.StreamQuantity == streamingQuantity && msg.StreamInterval == streamingInterval {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("streaming swap not found in queue with expected chunk parameters")
	}
}

func TestIntegrationSwap_RapidMatchOpposingStreams(t *testing.T) {
	env := setupIntegrationEnv(t)

	setupPoolForTest(t, env.Ctx, env.Keeper, common.BTCAsset)
	setupPoolForTest(t, env.Ctx, env.Keeper, common.ZECAsset)

	signerA := types.GetRandomBech32Addr()
	signerB := types.GetRandomBech32Addr()
	fundAccount(t, env.Ctx, env.Keeper, signerA, 100*common.One)
	fundAccount(t, env.Ctx, env.Keeper, signerB, 100*common.One)

	destA := types.GetRandomBTCAddress()
	destB := types.GetRandomBTCAddress()

	fromA, err := common.NewAddress(signerA.String())
	if err != nil {
		t.Fatalf("failed to create address A: %v", err)
	}
	fromB, err := common.NewAddress(signerB.String())
	if err != nil {
		t.Fatalf("failed to create address B: %v", err)
	}

	txA := common.NewTx(
		common.BlankTxID,
		fromA,
		common.Address(destA),
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(5*common.One))},
		common.Gas{},
		"swap-btc-to-zec",
	)
	swapA := types.NewMsgSwap(
		txA, common.ZECAsset, common.Address(destA),
		cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(),
		"", "", nil,
		types.SwapType_market, 3, 1, types.SwapVersion_v1, signerA,
	)

	txB := common.NewTx(
		common.BlankTxID,
		fromB,
		common.Address(destB),
		common.Coins{common.NewCoin(common.ZECAsset, cosmos.NewUint(5*common.One))},
		common.Gas{},
		"swap-zec-to-btc",
	)
	swapB := types.NewMsgSwap(
		txB, common.BTCAsset, common.Address(destB),
		cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(),
		"", "", nil,
		types.SwapType_market, 3, 1, types.SwapVersion_v1, signerB,
	)

	err = env.Keeper.SetSwapQueueItem(env.Ctx, *swapA, 0)
	if err != nil {
		t.Fatalf("failed to queue swap A: %v", err)
	}
	err = env.Keeper.SetSwapQueueItem(env.Ctx, *swapB, 1)
	if err != nil {
		t.Fatalf("failed to queue swap B: %v", err)
	}

	iter := env.Keeper.GetSwapQueueIterator(env.Ctx)
	defer iter.Close()

	count := 0
	for ; iter.Valid(); iter.Next() {
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 opposing swaps in queue, got %d", count)
	}
}
