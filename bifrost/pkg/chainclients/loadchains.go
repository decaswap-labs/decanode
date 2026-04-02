package chainclients

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/tss"

	"github.com/decaswap-labs/decanode/bifrost/metrics"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/ethereum"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/evm"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/gaia"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/shared/types"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/solana"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/tron"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/utxo"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/xrp"
	"github.com/decaswap-labs/decanode/bifrost/pubkeymanager"
	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/config"
)

// ChainClient exports the shared type.
type ChainClient = types.ChainClient

// LoadChains returns chain clients from chain configuration
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
		case common.ETHChain:
			return ethereum.NewClient(thorKeys, chain, server, thorchainBridge, m, pubKeyValidator, poolMgr)
		case common.AVAXChain, common.BSCChain, common.BASEChain, common.POLChain:
			return evm.NewEVMClient(thorKeys, chain, server, thorchainBridge, m, pubKeyValidator, poolMgr)
		case common.GAIAChain, common.NOBLEChain:
			return gaia.NewCosmosClient(thorKeys, chain, server, thorchainBridge, m, pubKeyValidator)
		case common.BTCChain, common.BCHChain, common.LTCChain, common.DOGEChain, common.ZECChain:
			return utxo.NewClient(thorKeys, chain, server, thorchainBridge, m)
		case common.TRONChain:
			return tron.NewTronClient(thorKeys, chain, server, thorchainBridge, m, pubKeyValidator, poolMgr)
		case common.XRPChain:
			return xrp.NewClient(thorKeys, chain, server, thorchainBridge, m)
		case common.SOLChain:
			return solana.NewSOLClient(thorKeys, chain, server, thorchainBridge, m, pubKeyValidator, poolMgr)
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

		// Not needed for Zcash bechause it doesn't use listunspent for utxo
		// retrieval, which is not supported by zebrad (replacement for the
		// deprecated zcashd)
		// trunk-ignore-all(golangci-lint/forcetypeassert)
		switch chain.ChainID {
		case common.BTCChain, common.BCHChain, common.LTCChain, common.DOGEChain:
			pubKeyValidator.RegisterCallback(client.(*utxo.Client).RegisterPublicKey)
		}
		chains[chain.ChainID] = client
	}

	// watch failed chains minutely and restart bifrost if any succeed init
	if len(failedChains) > 0 {
		go func() {
			tick := time.NewTicker(time.Minute)
			for range tick.C {
				for _, chain := range failedChains {
					ccfg := cfg[chain]
					ccfg.BlockScanner.DBPath = "" // in-memory db

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
