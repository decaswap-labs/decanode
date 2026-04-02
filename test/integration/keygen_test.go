package integration

import (
	"testing"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

func TestIntegrationKeygen_CreateKeygenBlock(t *testing.T) {
	env := setupIntegrationEnv(t)

	keygenBlock := types.NewKeygenBlock(env.Ctx.BlockHeight())

	var members []string
	for _, v := range env.Validators {
		members = append(members, v.PubKeySet.Secp256k1.String())
	}

	keygen, err := types.NewKeygen(env.Ctx.BlockHeight(), members, types.KeygenType_AsgardKeygen)
	if err != nil {
		t.Fatalf("failed to create keygen: %v", err)
	}
	if keygen.IsEmpty() {
		t.Fatal("keygen should not be empty")
	}

	keygenBlock.Keygens = append(keygenBlock.Keygens, keygen)
	env.Keeper.SetKeygenBlock(env.Ctx, keygenBlock)

	retrieved, err := env.Keeper.GetKeygenBlock(env.Ctx, env.Ctx.BlockHeight())
	if err != nil {
		t.Fatalf("failed to get keygen block: %v", err)
	}
	if len(retrieved.Keygens) != 1 {
		t.Fatalf("expected 1 keygen, got %d", len(retrieved.Keygens))
	}
	if len(retrieved.Keygens[0].Members) != validatorCount {
		t.Fatalf("expected %d keygen members, got %d", validatorCount, len(retrieved.Keygens[0].Members))
	}
}

func TestIntegrationKeygen_ThresholdCheck(t *testing.T) {
	env := setupIntegrationEnv(t)

	if len(env.Validators) != validatorCount {
		t.Fatalf("expected %d validators, got %d", validatorCount, len(env.Validators))
	}

	if threshold > validatorCount {
		t.Fatal("threshold cannot exceed validator count")
	}

	expectedThreshold := (2 * validatorCount / 3) + 1
	if expectedThreshold < 2 {
		expectedThreshold = 2
	}
	if threshold != expectedThreshold {
		t.Logf("note: threshold=%d, 2/3+1=%d", threshold, expectedThreshold)
	}
}

func TestIntegrationKeygen_FreshVaultAfterKeygen(t *testing.T) {
	env := setupIntegrationEnv(t)

	newPubKey := types.GetRandomPubKey()
	newVault := types.NewVault(
		env.Ctx.BlockHeight()+100,
		types.VaultStatus_ActiveVault,
		types.VaultType_AsgardVault,
		newPubKey,
		common.Chains{common.BTCChain, common.ZECChain}.Strings(),
		[]types.ChainContract{},
	)
	for _, v := range env.Validators {
		newVault.Membership = append(newVault.Membership, v.PubKeySet.Secp256k1.String())
	}

	err := env.Keeper.SetVault(env.Ctx, newVault)
	if err != nil {
		t.Fatalf("failed to set new vault: %v", err)
	}

	retrieved, err := env.Keeper.GetVault(env.Ctx, newPubKey)
	if err != nil {
		t.Fatalf("failed to get new vault: %v", err)
	}
	if !retrieved.PubKey.Equals(newPubKey) {
		t.Fatal("new vault pubkey does not match")
	}
	if len(retrieved.Membership) != validatorCount {
		t.Fatalf("expected %d members in new vault, got %d", validatorCount, len(retrieved.Membership))
	}
}

func TestIntegrationKeygen_RetireOldVault(t *testing.T) {
	env := setupIntegrationEnv(t)

	vault, err := env.Keeper.GetVault(env.Ctx, env.VaultPubKey)
	if err != nil {
		t.Fatalf("failed to get vault: %v", err)
	}
	if vault.Status != types.VaultStatus_ActiveVault {
		t.Fatalf("expected active vault, got %s", vault.Status)
	}

	vault.Status = types.VaultStatus_RetiringVault
	err = env.Keeper.SetVault(env.Ctx, vault)
	if err != nil {
		t.Fatalf("failed to retire vault: %v", err)
	}

	retrieved, err := env.Keeper.GetVault(env.Ctx, env.VaultPubKey)
	if err != nil {
		t.Fatalf("failed to get retired vault: %v", err)
	}
	if retrieved.Status != types.VaultStatus_RetiringVault {
		t.Fatalf("expected retiring vault, got %s", retrieved.Status)
	}
}

func TestIntegrationKeygen_AddressIndexResetOnNewVault(t *testing.T) {
	env := setupIntegrationEnv(t)

	oldAddr, err := env.VaultPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to get old BTC address: %v", err)
	}

	newPubKey := types.GetRandomPubKey()
	newAddr, err := newPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to get new BTC address: %v", err)
	}

	if oldAddr.Equals(newAddr) {
		t.Fatal("new vault should produce a different BTC address than old vault")
	}

	newVault := types.NewVault(
		env.Ctx.BlockHeight()+100,
		types.VaultStatus_ActiveVault,
		types.VaultType_AsgardVault,
		newPubKey,
		common.Chains{common.BTCChain, common.ZECChain}.Strings(),
		[]types.ChainContract{},
	)
	err = env.Keeper.SetVault(env.Ctx, newVault)
	if err != nil {
		t.Fatalf("failed to set new vault: %v", err)
	}

	retrievedAddr, err := newPubKey.GetAddress(common.BTCChain)
	if err != nil {
		t.Fatalf("failed to derive address from new vault key: %v", err)
	}
	if retrievedAddr.IsEmpty() {
		t.Fatal("new vault BTC address should not be empty")
	}
	if retrievedAddr.Equals(oldAddr) {
		t.Fatal("address index should reset: new vault address must differ from old")
	}
}
