package thorchain

import (
	"github.com/decaswap-labs/decanode/common"
	cosmos "github.com/decaswap-labs/decanode/common/cosmos"
	keeper "github.com/decaswap-labs/decanode/x/thorchain/keeper"
)

type DummyObserverManager struct{}

func NewDummyObserverManager() *DummyObserverManager {
	return &DummyObserverManager{}
}

func (m *DummyObserverManager) BeginBlock()                                                  {}
func (m *DummyObserverManager) EndBlock(ctx cosmos.Context, keeper keeper.Keeper)            {}
func (m *DummyObserverManager) AppendObserver(chain common.Chain, addrs []cosmos.AccAddress) {}
func (m *DummyObserverManager) List() []cosmos.AccAddress                                    { return nil }
