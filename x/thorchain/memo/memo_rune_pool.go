package thorchain

import (
	"strings"

	"github.com/decaswap-labs/decanode/common"
	cosmos "github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

// "pool+"

type DecaPoolDepositMemo struct {
	MemoBase
}

func (m DecaPoolDepositMemo) String() string {
	return m.string(false)
}

func (m DecaPoolDepositMemo) ShortString() string {
	return m.string(true)
}

func (m DecaPoolDepositMemo) string(short bool) string {
	return "pool+"
}

func NewDecaPoolDepositMemo() DecaPoolDepositMemo {
	return DecaPoolDepositMemo{
		MemoBase: MemoBase{TxType: TxDecaPoolDeposit},
	}
}

func (p *parser) ParseDecaPoolDepositMemo() (DecaPoolDepositMemo, error) {
	return NewDecaPoolDepositMemo(), nil
}

// "pool-:<basis-points>:<affiliate>:<affiliate-basis-points>"

type DecaPoolWithdrawMemo struct {
	MemoBase
	BasisPoints          cosmos.Uint
	AffiliateAddress     common.Address
	AffiliateBasisPoints cosmos.Uint
	AffiliateTHORName    *types.THORName
}

func (m DecaPoolWithdrawMemo) GetBasisPts() cosmos.Uint              { return m.BasisPoints }
func (m DecaPoolWithdrawMemo) GetAffiliateAddress() common.Address   { return m.AffiliateAddress }
func (m DecaPoolWithdrawMemo) GetAffiliateBasisPoints() cosmos.Uint  { return m.AffiliateBasisPoints }
func (m DecaPoolWithdrawMemo) GetAffiliateTHORName() *types.THORName { return m.AffiliateTHORName }

func (m DecaPoolWithdrawMemo) String() string {
	args := []string{TxDecaPoolWithdraw.String(), m.BasisPoints.String(), m.AffiliateAddress.String(), m.AffiliateBasisPoints.String()}
	return strings.Join(args, ":")
}

func NewDecaPoolWithdrawMemo(basisPoints cosmos.Uint, affAddr common.Address, affBps cosmos.Uint, tn types.THORName) DecaPoolWithdrawMemo {
	mem := DecaPoolWithdrawMemo{
		MemoBase:             MemoBase{TxType: TxDecaPoolWithdraw},
		BasisPoints:          basisPoints,
		AffiliateAddress:     affAddr,
		AffiliateBasisPoints: affBps,
	}
	if !tn.Owner.Empty() {
		mem.AffiliateTHORName = &tn
	}
	return mem
}

func (p *parser) ParseDecaPoolWithdrawMemo() (DecaPoolWithdrawMemo, error) {
	basisPoints := p.getUint(1, true, cosmos.ZeroInt().Uint64())
	affiliateAddress := p.getAddressWithKeeper(2, false, common.NoAddress, common.THORChain)
	tn := p.getTHORName(2, false, types.NewTHORName("", 0, nil), -1)
	affiliateBasisPoints := p.getUintWithMaxValue(3, false, 0, constants.MaxBasisPts)
	return NewDecaPoolWithdrawMemo(basisPoints, affiliateAddress, affiliateBasisPoints, tn), p.Error()
}
