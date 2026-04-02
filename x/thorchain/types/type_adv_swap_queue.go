package types

import (
	"github.com/decaswap-labs/decanode/common"
)

// AdvSwapQueueIndexItem represents an item in the advanced swap queue index
type AdvSwapQueueIndexItem struct {
	TxID  common.TxID
	Index int
}
