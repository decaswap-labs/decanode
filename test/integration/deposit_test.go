package integration

import (
	"testing"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

func TestIntegrationDeposit_VaultAddressExists(t *testing.T) {
	env := setupIntegrationEnv(t)

	vault, err := env.Keeper.GetVault(env.Ctx, env.VaultPubKey)
	if err != nil {
		t.Fatalf("failed to get vault: %v", err)
	}

	chains := vault.GetChains()
	foundBTC := false
	for _, c := range chains {
		if c.Equals(common.BTCChain) {
			foundBTC = true
			break
		}
	}
	if !foundBTC {
		t.Fatal("expected vault to contain BTC chain")
	}

	addr, err := env.VaultPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to get BTC address from vault pubkey: %v", err)
	}
	if addr.IsEmpty() {
		t.Fatal("expected non-empty BTC deposit address")
	}
}

func TestIntegrationDeposit_ObserveInboundCreditBalance(t *testing.T) {
	env := setupIntegrationEnv(t)

	setupPoolForTest(t, env.Ctx, env.Keeper, common.BTCAsset)

	depositor := types.GetRandomBTCAddress()
	depositAmount := cosmos.NewUint(1 * common.One)

	vaultAddr, err := env.VaultPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to get vault BTC address: %v", err)
	}

	observedTx := common.NewObservedTx(
		common.NewTx(
			types.GetRandomTxHash(),
			depositor,
			vaultAddr,
			common.Coins{common.NewCoin(common.BTCAsset, depositAmount)},
			common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10000))},
			"",
		),
		env.Ctx.BlockHeight(),
		env.VaultPubKey,
		env.Ctx.BlockHeight(),
	)

	voter := types.NewObservedTxVoter(observedTx.Tx.ID, []common.ObservedTx{observedTx})
	env.Keeper.SetObservedTxInVoter(env.Ctx, voter)

	stored, err := env.Keeper.GetObservedTxInVoter(env.Ctx, observedTx.Tx.ID)
	if err != nil {
		t.Fatalf("failed to get observed tx voter: %v", err)
	}
	if len(stored.Txs) != 1 {
		t.Fatalf("expected 1 observed tx, got %d", len(stored.Txs))
	}
	if !stored.Txs[0].Tx.Coins[0].Amount.Equal(depositAmount) {
		t.Fatalf("expected deposit amount %s, got %s", depositAmount, stored.Txs[0].Tx.Coins[0].Amount)
	}
}

func TestIntegrationDeposit_VaultHasZECChain(t *testing.T) {
	env := setupIntegrationEnv(t)

	vault, err := env.Keeper.GetVault(env.Ctx, env.VaultPubKey)
	if err != nil {
		t.Fatalf("failed to get vault: %v", err)
	}

	chains := vault.GetChains()
	foundZEC := false
	for _, c := range chains {
		if c.Equals(common.ZECChain) {
			foundZEC = true
			break
		}
	}
	if !foundZEC {
		t.Fatal("expected vault to contain ZEC chain")
	}

	addr, err := env.VaultPubKey.GetAddress(common.ZECChain)
	if err != nil {
		t.Fatalf("failed to get ZEC address from vault pubkey: %v", err)
	}
	if addr.IsEmpty() {
		t.Fatal("expected non-empty ZEC deposit address")
	}
}

func TestIntegrationDeposit_MultipleObservationsFromValidators(t *testing.T) {
	env := setupIntegrationEnv(t)

	setupPoolForTest(t, env.Ctx, env.Keeper, common.BTCAsset)

	depositor := types.GetRandomBTCAddress()
	depositAmount := cosmos.NewUint(5 * common.One)
	txHash := types.GetRandomTxHash()

	vaultAddr, err := env.VaultPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to get vault BTC address: %v", err)
	}

	tx := common.NewTx(
		txHash,
		depositor,
		vaultAddr,
		common.Coins{common.NewCoin(common.BTCAsset, depositAmount)},
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10000))},
		"",
	)

	var observations []common.ObservedTx
	for i := 0; i < threshold; i++ {
		obs := common.NewObservedTx(tx, env.Ctx.BlockHeight(), env.VaultPubKey, env.Ctx.BlockHeight())
		observations = append(observations, obs)
	}

	voter := types.NewObservedTxVoter(txHash, observations)
	env.Keeper.SetObservedTxInVoter(env.Ctx, voter)

	stored, err := env.Keeper.GetObservedTxInVoter(env.Ctx, txHash)
	if err != nil {
		t.Fatalf("failed to get observed tx voter: %v", err)
	}
	if len(stored.Txs) < threshold {
		t.Fatalf("expected at least %d observations, got %d", threshold, len(stored.Txs))
	}
}
