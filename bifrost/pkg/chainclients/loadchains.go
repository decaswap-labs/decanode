package chainclients

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/tss"

	"github.com/decaswap-labs/decanode/bifrost/metrics"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/shared/types"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/utxo"
	"github.com/decaswap-labs/decanode/bifrost/pubkeymanager"
	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/config"
)

type ChainClient = types.ChainClient

func LoadChains(thorKeys *thorclient.Keys,
	cfg map[common.Chain]config.BifrostChainConfiguration,
	server *tss.TssServer,
	thorchainBridge thorclient.ThorchainBridge,
	m *metrics.Metrics,
	pubKeyValidator pubkeymanager.PubKeyValidator,
	poolMgr thorclient.PoolManager,
) (chains map[common.Chain]ChainClient, restart chan struct{}) {
	logger := log.Logger.With().Str("module", "bifrost").Logger()

	chains = make(map[common.Chain]ChainClient)
	restart = make(chan struct{})
	failedChains := []common.Chain{}

	loadChain := func(chain config.BifrostChainConfiguration) (ChainClient, error) {
		switch chain.ChainID {
		case common.BTCChain, common.ZECChain:
			return utxo.NewClient(thorKeys, chain, server, thorchainBridge, m)
		default:
			log.Fatal().Msgf("chain %s is not supported", chain.ChainID)
			return nil, nil
		}
	}

	for _, chain := range cfg {
		if chain.Disabled {
			logger.Info().Msgf("%s chain is disabled by configure", chain.ChainID)
			continue
		}

		client, err := loadChain(chain)
		if err != nil {
			logger.Error().Err(err).Stringer("chain", chain.ChainID).Msg("failed to load chain")
			failedChains = append(failedChains, chain.ChainID)
			continue
		}

		switch chain.ChainID {
		case common.BTCChain:
			pubKeyValidator.RegisterCallback(client.(*utxo.Client).RegisterPublicKey)
		}
		chains[chain.ChainID] = client
	}

	if len(failedChains) > 0 {
		go func() {
			tick := time.NewTicker(time.Minute)
			for range tick.C {
				for _, chain := range failedChains {
					ccfg := cfg[chain]
					ccfg.BlockScanner.DBPath = ""

					_, err := loadChain(ccfg)
					if err == nil {
						logger.Info().Stringer("chain", chain).Msg("chain loaded, restarting bifrost")
						close(restart)
						return
					} else {
						logger.Error().Err(err).Stringer("chain", chain).Msg("failed to load chain")
					}
				}
			}
		}()
	}

	return chains, restart
}
