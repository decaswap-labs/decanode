package watchers

import (
	"fmt"
	"time"

	. "github.com/decaswap-labs/decanode/test/simulation/pkg/types"
	"github.com/decaswap-labs/decanode/tools/thorscan"
)

func NewSecurityEvents() *Watcher {
	thorscan.SetAPIEndpoint(thornodeURL)
	scanner := thorscan.Scan(1, 0)

	return &Watcher{
		Name:     "SecurityEvents",
		Interval: time.Second,
		Fn: func(config *OpConfig) error {
			for {
				select {
				case block := <-scanner:
					events := block.FinalizeBlockEvents
					events = append(events, block.BeginBlockEvents...)
					events = append(events, block.EndBlockEvents...)
					for _, tx := range block.Txs {
						events = append(events, tx.Result.Events...)
					}
					for _, event := range events {
						if event["type"] == "security" {
							return fmt.Errorf("security event detected at height %d: %v", block.Header.Height, event)
						}
					}
				case <-time.After(10 * time.Millisecond):
					return nil
				}
			}
		},
	}
}
