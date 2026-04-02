//go:build mainnet
// +build mainnet

package thorchain

import (
	"github.com/decaswap-labs/decanode/common/cosmos"
)

// processMigration handles the actual migration logic.
func (m *ScheduledMigrationMgr) processMigration(ctx cosmos.Context, mgr Manager) error {
	return nil
}
