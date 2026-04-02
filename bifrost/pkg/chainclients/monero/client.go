package monero

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	stdatomic "sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/config"
)

type Client struct {
	cfg    config.BifrostChainConfiguration
	bridge thorclient.ThorchainBridge
	logger zerolog.Logger

	daemonRPC *DaemonRPC
	scanner   *Scanner

	wg       *sync.WaitGroup
	stopchan chan struct{}

	currentBlockHeight stdatomic.Int64
	scannerHealthy     stdatomic.Bool
}

func NewClient(
	cfg config.BifrostChainConfiguration,
	bridge thorclient.ThorchainBridge,
) (*Client, error) {
	if cfg.RPCHost == "" {
		return nil, errors.New("monero daemon RPC host is empty")
	}

	logger := log.Logger.With().Stringer("chain", cfg.ChainID).Logger()
	daemonRPC := NewDaemonRPC(cfg.RPCHost)

	c := &Client{
		cfg:       cfg,
		bridge:    bridge,
		logger:    logger,
		daemonRPC: daemonRPC,
		scanner:   NewScanner(daemonRPC, logger),
		wg:        &sync.WaitGroup{},
		stopchan:  make(chan struct{}),
	}

	c.scannerHealthy.Store(true)

	return c, nil
}

func (c *Client) Start(
	globalTxsQueue chan types.TxIn,
	globalErrataQueue chan types.ErrataBlock,
	globalSolvencyQueue chan types.Solvency,
	globalNetworkFeeQueue chan common.NetworkFee,
) {
	c.wg.Add(1)
	go c.scanBlocks(globalTxsQueue)
}

func (c *Client) scanBlocks(txsQueue chan types.TxIn) {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopchan:
			return
		default:
		}

		height, err := c.daemonRPC.GetHeight()
		if err != nil {
			c.logger.Error().Err(err).Msg("failed to get monero block height")
			c.scannerHealthy.Store(false)
			continue
		}

		c.currentBlockHeight.Store(height)
		c.scannerHealthy.Store(true)
	}
}

func (c *Client) Stop() {
	close(c.stopchan)
	c.wg.Wait()
}

func (c *Client) GetChain() common.Chain {
	return common.XMRChain
}

func (c *Client) GetConfig() config.BifrostChainConfiguration {
	return c.cfg
}

func (c *Client) IsBlockScannerHealthy() bool {
	return c.scannerHealthy.Load()
}

func (c *Client) GetHeight() (int64, error) {
	return c.daemonRPC.GetHeight()
}

func (c *Client) GetAddress(_ common.PubKey) string {
	return ""
}

func (c *Client) GetAccount(_ common.PubKey, _ *big.Int) (common.Account, error) {
	return common.Account{}, nil
}

func (c *Client) GetAccountByAddress(_ string, _ *big.Int) (common.Account, error) {
	return common.Account{}, nil
}

func (c *Client) SignTx(_ types.TxOutItem, _ int64) ([]byte, []byte, *types.TxInItem, error) {
	return nil, nil, nil, fmt.Errorf("XMR signing not yet implemented")
}

func (c *Client) BroadcastTx(_ types.TxOutItem, _ []byte) (string, error) {
	return "", fmt.Errorf("XMR broadcast not yet implemented")
}

func (c *Client) OnObservedTxIn(_ types.TxInItem, _ int64) {
}

func (c *Client) GetConfirmationCount(txIn types.TxIn) int64 {
	if len(txIn.TxArray) == 0 {
		return 0
	}
	if txIn.MemPool {
		return 0
	}
	return 10
}

func (c *Client) ConfirmationCountReady(txIn types.TxIn) bool {
	if len(txIn.TxArray) == 0 {
		return true
	}
	if txIn.MemPool {
		return true
	}

	height := txIn.TxArray[0].BlockHeight
	confirm := txIn.ConfirmationRequired
	ready := (c.currentBlockHeight.Load() - height) >= confirm
	return ready
}

func (c *Client) GetBlockScannerHeight() (int64, error) {
	return c.currentBlockHeight.Load(), nil
}

func (c *Client) GetLatestTxForVault(_ string) (string, string, error) {
	return "", "", nil
}

func (c *Client) RollbackBlockScanner() error {
	return nil
}
