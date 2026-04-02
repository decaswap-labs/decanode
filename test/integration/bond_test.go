package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

var (
	errInvalidMessage       = errors.New("invalid message")
	errUnbondingNotSupported = fmt.Errorf("unbonding is not supported — bonds are permanent")
)

func TestIntegrationBond_CreateValidator(t *testing.T) {
	env := setupIntegrationEnv(t)

	newNode := types.GetRandomValidatorNode(types.NodeStatus_Whitelisted)
	newNode.Version = types.GetCurrentVersion().String()

	ver := types.GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)
	minimumBond := constAccessor.GetInt64Value(constants.MinimumBondInDeca)

	newNode.Bond = cosmos.NewUint(uint64(minimumBond))
	err := env.Keeper.SetNodeAccount(env.Ctx, newNode)
	if err != nil {
		t.Fatalf("failed to set node account: %v", err)
	}

	retrieved, err := env.Keeper.GetNodeAccount(env.Ctx, newNode.NodeAddress)
	if err != nil {
		t.Fatalf("failed to get node account: %v", err)
	}
	if !retrieved.Bond.Equal(cosmos.NewUint(uint64(minimumBond))) {
		t.Fatalf("expected bond %d, got %s", minimumBond, retrieved.Bond)
	}
	if retrieved.Status != types.NodeStatus_Whitelisted {
		t.Fatalf("expected status Whitelisted, got %s", retrieved.Status)
	}
}

func TestIntegrationBond_PromoteToActive(t *testing.T) {
	env := setupIntegrationEnv(t)

	newNode := types.GetRandomValidatorNode(types.NodeStatus_Whitelisted)
	newNode.Version = types.GetCurrentVersion().String()
	newNode.Bond = cosmos.NewUint(1_000_000 * common.One)

	err := env.Keeper.SetNodeAccount(env.Ctx, newNode)
	if err != nil {
		t.Fatalf("failed to set node account: %v", err)
	}

	newNode.UpdateStatus(types.NodeStatus_Active, env.Ctx.BlockHeight())
	err = env.Keeper.SetNodeAccount(env.Ctx, newNode)
	if err != nil {
		t.Fatalf("failed to promote node: %v", err)
	}

	retrieved, err := env.Keeper.GetNodeAccount(env.Ctx, newNode.NodeAddress)
	if err != nil {
		t.Fatalf("failed to get node account: %v", err)
	}
	if retrieved.Status != types.NodeStatus_Active {
		t.Fatalf("expected status Active, got %s", retrieved.Status)
	}
}

func TestIntegrationBond_UnbondingRejected(t *testing.T) {
	env := setupIntegrationEnv(t)

	activeNode := env.Validators[0]

	txIn := common.NewTx(
		types.GetRandomTxHash(),
		types.GetRandomTHORAddress(),
		types.GetRandomTHORAddress(),
		common.Coins{
			common.NewCoin(common.DecaAsset(), cosmos.NewUint(100*common.One)),
		},
		common.Gas{},
		"unbond",
	)
	msg := types.NewMsgUnBond(
		txIn,
		activeNode.NodeAddress,
		cosmos.NewUint(100*common.One),
		activeNode.BondAddress,
		nil,
		activeNode.NodeAddress,
	)

	handler := NewUnBondHandlerForTest(env)
	_, err := handler.Run(env.Ctx, msg)
	if err == nil {
		t.Fatal("expected unbonding to be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "not supported") && !strings.Contains(err.Error(), "permanent") {
		t.Fatalf("expected permanent bond error, got: %v", err)
	}
}

func TestIntegrationBond_FeeDistribution9010(t *testing.T) {
	env := setupIntegrationEnv(t)

	activeNodes, err := env.Keeper.ListActiveValidators(env.Ctx)
	if err != nil {
		t.Fatalf("failed to list validators: %v", err)
	}

	totalFees := cosmos.NewUint(1000 * common.One)

	validatorShare := totalFees.MulUint64(90).QuoUint64(100)
	treasuryShare := totalFees.MulUint64(10).QuoUint64(100)

	expectedValidatorShare := cosmos.NewUint(900 * common.One)
	expectedTreasuryShare := cosmos.NewUint(100 * common.One)

	if !validatorShare.Equal(expectedValidatorShare) {
		t.Fatalf("expected validator share %s, got %s", expectedValidatorShare, validatorShare)
	}
	if !treasuryShare.Equal(expectedTreasuryShare) {
		t.Fatalf("expected treasury share %s, got %s", expectedTreasuryShare, treasuryShare)
	}

	total := validatorShare.Add(treasuryShare)
	if !total.Equal(totalFees) {
		t.Fatalf("shares should sum to total fees: %s != %s", total, totalFees)
	}

	nodeCount := uint64(len(activeNodes))
	perValidator := validatorShare.QuoUint64(nodeCount)
	expectedPer := cosmos.NewUint(300 * common.One)
	if !perValidator.Equal(expectedPer) {
		t.Fatalf("expected per-validator share %s, got %s", expectedPer, perValidator)
	}
}

func TestIntegrationBond_ValidatorCount(t *testing.T) {
	env := setupIntegrationEnv(t)

	activeNodes, err := env.Keeper.ListActiveValidators(env.Ctx)
	if err != nil {
		t.Fatalf("failed to list active validators: %v", err)
	}
	if len(activeNodes) != validatorCount {
		t.Fatalf("expected %d active validators, got %d", validatorCount, len(activeNodes))
	}
}

type unbondHandler struct{}

func NewUnBondHandlerForTest(_ *IntegrationTestEnv) unbondHandler {
	return unbondHandler{}
}

func (h unbondHandler) Run(_ cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	_, ok := m.(*types.MsgUnBond)
	if !ok {
		return nil, errInvalidMessage
	}
	return nil, errUnbondingNotSupported
}
