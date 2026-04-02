package thorchain

import (
	"github.com/decaswap-labs/decanode/common/cosmos"
)

type DummyPoolManager struct{}

func NewDummyPoolManager() *DummyPoolManager {
	return &DummyPoolManager{}
}

func (m *DummyPoolManager) EndBlock(ctx cosmos.Context, mgr Manager) error {
	return nil
}
