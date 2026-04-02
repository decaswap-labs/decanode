//go:build !stagenet && !chainnet && !mainnet
// +build !stagenet,!chainnet,!mainnet

package thorchain

import "github.com/decaswap-labs/decanode/common/cosmos"

// processMigration handles the actual migration logic.
func (m *ScheduledMigrationMgr) processMigration(ctx cosmos.Context, mgr Manager) error {
	mgr.Keeper().SetMimir(ctx, "SCHEDULED-MIGRATION-MOCKNET", 123)
	return nil
}
