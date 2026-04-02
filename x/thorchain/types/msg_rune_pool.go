package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
)

var (
	_ sdk.Msg              = &MsgDecaPoolDeposit{}
	_ sdk.HasValidateBasic = &MsgDecaPoolDeposit{}
	_ sdk.LegacyMsg        = &MsgDecaPoolDeposit{}

	_ sdk.Msg              = &MsgDecaPoolWithdraw{}
	_ sdk.HasValidateBasic = &MsgDecaPoolWithdraw{}
	_ sdk.LegacyMsg        = &MsgDecaPoolWithdraw{}
)

// NewMsgDecaPoolDeposit create new MsgDecaPoolDeposit message
func NewMsgDecaPoolDeposit(signer cosmos.AccAddress, tx common.Tx) *MsgDecaPoolDeposit {
	return &MsgDecaPoolDeposit{
		Signer: signer,
		Tx:     tx,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgDecaPoolDeposit) ValidateBasic() error {
	if !m.Tx.Chain.Equals(common.THORChain) {
		return cosmos.ErrUnauthorized("chain must be THORChain")
	}
	if len(m.Tx.Coins) != 1 {
		return cosmos.ErrInvalidCoins("coins must be length 1 (RUNE)")
	}
	if !m.Tx.Coins[0].Asset.Chain.IsTHORChain() {
		return cosmos.ErrInvalidCoins("coin chain must be THORChain")
	}
	if !m.Tx.Coins[0].IsDeca() {
		return cosmos.ErrInvalidCoins("coin must be RUNE")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer must not be empty")
	}
	if m.Tx.Coins[0].Amount.IsZero() {
		return cosmos.ErrUnknownRequest("coins amount must not be zero")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgDecaPoolDeposit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

// NewMsgDecaPoolWithdraw create new MsgDecaPoolWithdraw message
func NewMsgDecaPoolWithdraw(signer cosmos.AccAddress, tx common.Tx, basisPoints cosmos.Uint, affAddr common.Address, affBps cosmos.Uint) *MsgDecaPoolWithdraw {
	return &MsgDecaPoolWithdraw{
		Signer:               signer,
		Tx:                   tx,
		BasisPoints:          basisPoints,
		AffiliateAddress:     affAddr,
		AffiliateBasisPoints: affBps,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgDecaPoolWithdraw) ValidateBasic() error {
	if !m.Tx.Coins.IsEmpty() {
		return cosmos.ErrInvalidCoins("coins must be empty (zero amount)")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer must not be empty")
	}
	if m.BasisPoints.IsZero() || m.BasisPoints.GT(cosmos.NewUint(constants.MaxBasisPts)) {
		return cosmos.ErrUnknownRequest("invalid basis points")
	}
	if m.AffiliateBasisPoints.GT(cosmos.NewUint(constants.MaxBasisPts)) {
		return cosmos.ErrUnknownRequest("invalid affiliate basis points")
	}
	if !m.AffiliateBasisPoints.IsZero() && m.AffiliateAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("affiliate basis points with no affiliate address")
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgDecaPoolWithdraw) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
