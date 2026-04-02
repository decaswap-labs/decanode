package thorchain

import (
	"github.com/cometbft/cometbft/crypto"

	"github.com/decaswap-labs/decanode/common/cosmos"
)

type nodeAddressValidatorAddressPair struct {
	nodeAddress      cosmos.AccAddress
	validatorAddress crypto.Address
}
