package integration

import (
	"os"
	"testing"
	"time"

	sdklog "cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
	"github.com/decaswap-labs/decanode/x/thorchain/keeper"
	kv1 "github.com/decaswap-labs/decanode/x/thorchain/keeper/v1"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

const (
	validatorCount = 3
	threshold      = 2
)

type IntegrationTestEnv struct {
	T          *testing.T
	Ctx        cosmos.Context
	Keeper     keeper.Keeper
	BankKeeper bankkeeper.Keeper
	Validators []types.NodeAccount
	VaultPubKey common.PubKey
}

func setupIntegrationEnv(t *testing.T) *IntegrationTestEnv {
	t.Helper()
	os.Setenv("NET", "mocknet")

	types.SetupConfigForTest()

	keyAcc := cosmos.NewKVStoreKey(authtypes.StoreKey)
	keyBank := cosmos.NewKVStoreKey(banktypes.StoreKey)
	keyUpgrade := cosmos.NewKVStoreKey(upgradetypes.StoreKey)
	keyThorchain := storetypes.NewKVStoreKey(types.StoreKey)
	serviceThorchain := runtime.NewKVStoreService(keyThorchain)

	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, sdklog.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(keyAcc, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyBank, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyUpgrade, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyThorchain, cosmos.StoreTypeIAVL, db)
	err := ms.LoadLatestVersion()
	if err != nil {
		t.Fatal(err)
	}

	ctx := cosmos.NewContext(ms, tmproto.Header{ChainID: "thorchain"}, false, sdklog.NewNopLogger())
	ctx = ctx.WithBlockHeight(18)
	ctx = ctx.WithBlockTime(time.Now())
	ctx = ctx.WithTxBytes([]byte("tx"))

	encodingConfig := testutil.MakeTestEncodingConfig(
		bank.AppModuleBasic{},
		auth.AppModuleBasic{},
	)

	ak := authkeeper.NewAccountKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyAcc),
		authtypes.ProtoBaseAccount,
		map[string][]string{
			types.ModuleName:  {authtypes.Minter, authtypes.Burner},
			types.AsgardName:  {},
			types.BondName:    {authtypes.Staking},
			types.ReserveName: {},
			types.TreasuryName: {},
			types.DECAPoolName: {},
		},
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress(types.ModuleName).String(),
	)

	bk := bankkeeper.NewBaseKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyBank),
		ak,
		nil,
		authtypes.NewModuleAddress(types.ModuleName).String(),
		sdklog.NewNopLogger(),
	)

	err = bk.MintCoins(ctx, types.ModuleName, cosmos.Coins{
		cosmos.NewCoin(common.DecaAsset().Native(), cosmos.NewInt(200_000_000_00000000)),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = bk.BurnCoins(ctx, types.ModuleName, cosmos.Coins{
		cosmos.NewCoin(common.DecaAsset().Native(), cosmos.NewInt(200_000_000_00000000)),
	})
	if err != nil {
		t.Fatal(err)
	}

	uk := upgradekeeper.NewKeeper(
		nil,
		runtime.NewKVStoreService(keyUpgrade),
		encodingConfig.Codec,
		t.TempDir(),
		nil,
		authtypes.NewModuleAddress(types.ModuleName).String(),
	)

	k := kv1.NewKVStore(encodingConfig.Codec, serviceThorchain, bk, ak, uk, types.GetCurrentVersion())
	kpr := keeper.Keeper(&k)

	fundModule(t, ctx, kpr, types.ModuleName, 1_000_000*common.One)
	fundModule(t, ctx, kpr, types.AsgardName, 100_000_000*common.One)
	fundModule(t, ctx, kpr, types.ReserveName, 10_000*common.One)

	err = kpr.SaveNetworkFee(ctx, common.BTCChain, types.NetworkFee{
		Chain:              common.BTCChain,
		TransactionSize:    1,
		TransactionFeeRate: 6423600,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = kpr.SaveNetworkFee(ctx, common.ZECChain, types.NetworkFee{
		Chain:              common.ZECChain,
		TransactionSize:    1,
		TransactionFeeRate: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	validators := setupValidators(t, ctx, kpr)

	vaultPubKey := types.GetRandomPubKey()
	vault := types.NewVault(
		ctx.BlockHeight(),
		types.VaultStatus_ActiveVault,
		types.VaultType_AsgardVault,
		vaultPubKey,
		common.Chains{common.BTCChain, common.ZECChain}.Strings(),
		[]types.ChainContract{},
	)
	for _, v := range validators {
		vault.Membership = append(vault.Membership, v.PubKeySet.Secp256k1.String())
	}
	err = kpr.SetVault(ctx, vault)
	if err != nil {
		t.Fatal(err)
	}

	return &IntegrationTestEnv{
		T:           t,
		Ctx:         ctx,
		Keeper:      kpr,
		BankKeeper:  bk,
		Validators:  validators,
		VaultPubKey: vaultPubKey,
	}
}

func setupValidators(t *testing.T, ctx cosmos.Context, k keeper.Keeper) []types.NodeAccount {
	t.Helper()

	constants.SWVersion = types.GetCurrentVersion()
	validators := make([]types.NodeAccount, validatorCount)
	for i := 0; i < validatorCount; i++ {
		na := types.GetRandomValidatorNode(types.NodeStatus_Active)
		na.Version = types.GetCurrentVersion().String()
		na.Bond = cosmos.NewUint(1_000_000 * common.One)
		na.ActiveBlockHeight = 10
		err := k.SetNodeAccount(ctx, na)
		if err != nil {
			t.Fatal(err)
		}
		validators[i] = na
	}
	return validators
}

func fundModule(t *testing.T, ctx cosmos.Context, k keeper.Keeper, name string, amt uint64) {
	t.Helper()
	coin := common.NewCoin(common.DecaNative, cosmos.NewUint(amt))
	err := k.MintToModule(ctx, types.ModuleName, coin)
	if err != nil {
		t.Fatal(err)
	}
	err = k.SendFromModuleToModule(ctx, types.ModuleName, name, common.NewCoins(coin))
	if err != nil {
		t.Fatal(err)
	}
}

func fundAccount(t *testing.T, ctx cosmos.Context, k keeper.Keeper, addr cosmos.AccAddress, amt uint64) {
	t.Helper()
	coin := common.NewCoin(common.DecaNative, cosmos.NewUint(amt))
	err := k.MintToModule(ctx, types.ModuleName, coin)
	if err != nil {
		t.Fatal(err)
	}
	err = k.SendFromModuleToAccount(ctx, types.ModuleName, addr, common.NewCoins(coin))
	if err != nil {
		t.Fatal(err)
	}
}

func setupPoolForTest(t *testing.T, ctx cosmos.Context, k keeper.Keeper, asset common.Asset) {
	t.Helper()
	pool := types.NewPool()
	pool.Asset = asset
	pool.Status = types.PoolStatus_Available
	pool.BalanceDeca = cosmos.NewUint(1_000_000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	err := k.SetPool(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
}
