package monero

import (
	"github.com/rs/zerolog"

	"github.com/decaswap-labs/decanode/bifrost/thorclient/types"
)

type Scanner struct {
	daemonRPC      *DaemonRPC
	logger         zerolog.Logger
	watchAddresses map[string]bool
}

func NewScanner(daemonRPC *DaemonRPC, logger zerolog.Logger) *Scanner {
	return &Scanner{
		daemonRPC:      daemonRPC,
		logger:         logger.With().Str("module", "xmr-scanner").Logger(),
		watchAddresses: make(map[string]bool),
	}
}

func (s *Scanner) AddWatchAddress(addr string) {
	s.watchAddresses[addr] = true
}

func (s *Scanner) RemoveWatchAddress(addr string) {
	delete(s.watchAddresses, addr)
}

func (s *Scanner) ScanBlock(height int64) ([]types.TxInItem, error) {
	_, err := s.daemonRPC.GetBlock(height)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
