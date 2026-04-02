package utxo

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	btctxscript "github.com/decaswap-labs/decanode/bifrost/txscript/txscript"

	btypes "github.com/decaswap-labs/decanode/bifrost/blockscanner/types"
	"github.com/decaswap-labs/decanode/bifrost/metrics"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/shared/utxo"
	"github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
	mem "github.com/decaswap-labs/decanode/x/thorchain/memo"
)

////////////////////////////////////////////////////////////////////////////////////////
// Address Checks
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) getAsgardAddress() ([]common.Address, error) {
	return utxo.GetAsgardAddressCached(&c.asgardCache, c.cfg.ChainID, c.bridge, constants.ThorchainBlockTime)
}

func (c *Client) isAsgardAddress(addressToCheck string) bool {
	asgards, err := c.getAsgardAddress()
	if err != nil {
		c.log.Err(err).Msg("fail to get asgard addresses")
		if len(asgards) == 0 {
			return false
		}
	}
	for _, addr := range asgards {
		if strings.EqualFold(addr.String(), addressToCheck) {
			return true
		}
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////////////
// Reorg Handling
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) processReorg(block *btcjson.GetBlockVerboseTxResult) ([]types.TxIn, error) {
	previousHeight := block.Height - 1
	prevBlockMeta, err := c.temporalStorage.GetBlockMeta(previousHeight)
	if err != nil {
		return nil, fmt.Errorf("fail to get block meta of height(%d): %w", previousHeight, err)
	}
	if prevBlockMeta == nil {
		return nil, nil
	}
	if strings.EqualFold(prevBlockMeta.BlockHash, block.PreviousHash) {
		return nil, nil
	}

	c.log.Info().
		Int64("currentHeight", block.Height).
		Str("previousHash", block.PreviousHash).
		Int64("blockMetaHeight", prevBlockMeta.Height).
		Str("blockMetaHash", prevBlockMeta.BlockHash).
		Msg("re-org detected")

	blockHeights, err := c.reConfirmTx(block.Height)
	if err != nil {
		c.log.Err(err).Msgf("fail to reprocess all txs")
	}
	var txIns []types.TxIn
	for _, height := range blockHeights {
		c.log.Info().Int64("height", height).Msg("rescanning block")
		var b *btcjson.GetBlockVerboseTxResult
		b, err = c.getBlock(height)
		if err != nil {
			c.log.Err(err).Int64("height", height).Msg("fail to get block from RPC")
			continue
		}
		var txIn types.TxIn
		txIn, err = c.extractTxs(b)
		if err != nil {
			c.log.Err(err).Msgf("fail to extract txIn from block")
			continue
		}
		if len(txIn.TxArray) == 0 {
			continue
		}
		txIns = append(txIns, txIn)
	}
	return txIns, nil
}

func (c *Client) reConfirmTx(height int64) ([]int64, error) {
	var rescanBlockHeights []int64

	earliestHeight := height - c.cfg.BlockScanner.MaxReorgRescanBlocks
	if earliestHeight < 1 {
		earliestHeight = 1
	}

	for i := height - 1; i >= earliestHeight; i-- {
		blockMeta, err := c.temporalStorage.GetBlockMeta(i)
		if err != nil {
			return nil, fmt.Errorf("fail to get block meta %d from local storage: %w", i, err)
		}

		hash, err := c.rpc.GetBlockHash(blockMeta.Height)
		if err != nil {
			c.log.Err(err).Msgf("fail to get block verbose tx result: %d", blockMeta.Height)
		}
		if strings.EqualFold(blockMeta.BlockHash, hash) {
			break
		}

		c.log.Info().Int64("height", blockMeta.Height).Msg("re-confirming transactions")

		var errataTxs []types.ErrataTx
		for _, tx := range blockMeta.CustomerTransactions {
			if c.confirmTx(tx) {
				c.log.Info().Int64("height", blockMeta.Height).Str("txid", tx).Msg("transaction still exists")
				continue
			}

			c.log.Info().Int64("height", blockMeta.Height).Str("txid", tx).Msg("errata tx")
			errataTxs = append(errataTxs, types.ErrataTx{
				TxID:  common.TxID(tx),
				Chain: c.cfg.ChainID,
			})

			blockMeta.RemoveCustomerTransaction(tx)
		}

		if len(errataTxs) > 0 {
			c.globalErrataQueue <- types.ErrataBlock{
				Height: blockMeta.Height,
				Txs:    errataTxs,
			}
		}

		rescanBlockHeights = append(rescanBlockHeights, blockMeta.Height)

		var r *btcjson.GetBlockVerboseResult
		r, err = c.rpc.GetBlockVerbose(hash)
		if err != nil {
			c.log.Err(err).Int64("height", blockMeta.Height).Msg("fail to get block verbose result")
		}
		blockMeta.PreviousHash = r.PreviousHash
		blockMeta.BlockHash = r.Hash
		err = c.temporalStorage.SaveBlockMeta(blockMeta.Height, blockMeta)
		if err != nil {
			c.log.Err(err).Int64("height", blockMeta.Height).Msg("fail to save block meta of height")
		}
	}
	return rescanBlockHeights, nil
}

func (c *Client) confirmTx(txid string) bool {
	_, err := c.rpc.GetRawTransaction(txid)
	if err != nil {
		c.log.Err(err).Str("txid", txid).Msg("fail to get tx")
	}
	return err == nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Mempool Cache
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) removeFromMemPoolCache(hash string) {
	err := c.temporalStorage.UntrackMempoolTx(hash)
	if err != nil {
		c.log.Err(err).Str("txid", hash).Msg("fail to remove from mempool cache")
	}
}

func (c *Client) tryAddToMemPoolCache(hash string) bool {
	added, err := c.temporalStorage.TrackMempoolTx(hash)
	if err != nil {
		c.log.Err(err).Str("txid", hash).Msg("fail to add to mempool cache")
	}
	return added
}

func (c *Client) canDeleteBlock(blockMeta *utxo.BlockMeta) bool {
	if blockMeta == nil {
		return true
	}
	for _, tx := range blockMeta.SelfTransactions {
		result, err := c.rpc.GetMempoolEntry(tx)
		if err == nil && result != nil {
			c.log.Info().Str("txid", tx).Msg("still in mempool, block cannot be deleted")
			return false
		}
	}
	return true
}

func (c *Client) updateNetworkInfo() {
	networkInfo, err := c.rpc.GetNetworkInfo()
	if err != nil {
		c.log.Err(err).Msg("fail to get network info")
		return
	}
	amt, err := btcutil.NewAmount(networkInfo.RelayFee)
	if err != nil {
		c.log.Err(err).Msg("fail to get minimum relay fee")
		return
	}
	c.minRelayFeeSats.Store(uint64(amt.ToUnit(btcutil.AmountSatoshi)))
}

func (c *Client) sendNetworkFee(height int64) error {
	var feeRate uint64
	switch c.cfg.ChainID {
	case common.BTCChain:
		hash, err := c.rpc.GetBlockHash(height)
		if err != nil {
			return fmt.Errorf("fail to get block hash: %w", err)
		}
		bs, err := c.rpc.GetBlockStats(hash)
		if err != nil {
			return fmt.Errorf("fail to get block stats: %w", err)
		}
		feeRate = uint64(bs.AverageFeeRate)

	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	if feeRate == 0 {
		return nil
	}

	c.networkFeeLock.Lock()
	defer c.networkFeeLock.Unlock()

	minRelayFeeSats := c.minRelayFeeSats.Load()
	if c.cfg.UTXO.EstimatedAverageTxSize*feeRate < minRelayFeeSats {
		feeRate = minRelayFeeSats / c.cfg.UTXO.EstimatedAverageTxSize
		if feeRate*c.cfg.UTXO.EstimatedAverageTxSize < minRelayFeeSats {
			feeRate++
		}
	}

	if feeRate < uint64(c.cfg.UTXO.MinSatsPerVByte) {
		feeRate = uint64(c.cfg.UTXO.MinSatsPerVByte)
	}

	if c.cfg.BlockScanner.GasCacheBlocks > 0 {
		c.feeRateCache = append(c.feeRateCache, feeRate)
		if len(c.feeRateCache) > c.cfg.BlockScanner.GasCacheBlocks {
			c.feeRateCache = c.feeRateCache[len(c.feeRateCache)-c.cfg.BlockScanner.GasCacheBlocks:]
		}
		for _, rate := range c.feeRateCache {
			if rate > feeRate {
				feeRate = rate
			}
		}
	}

	c.m.GetGauge(metrics.GasPrice(c.cfg.ChainID)).Set(float64(feeRate))

	if c.lastFeeRate.Load() == feeRate {
		return nil
	}

	c.lastFeeRate.Store(feeRate)
	c.m.GetCounter(metrics.GasPriceChange(c.cfg.ChainID)).Inc()

	c.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           c.cfg.ChainID,
		Height:          height,
		TransactionSize: c.cfg.UTXO.EstimatedAverageTxSize,
		TransactionRate: feeRate,
	}

	c.log.Debug().Msg("send network fee to THORNode successfully")
	return nil
}

func (c *Client) sendNetworkFeeFromBlock(blockResult *btcjson.GetBlockVerboseTxResult) error {
	height := blockResult.Height
	var total float64
	var totalVSize int32
	for _, tx := range blockResult.Tx {
		if len(tx.Vin) == 1 && tx.Vin[0].IsCoinBase() {
			for _, opt := range tx.Vout {
				total += opt.Value
			}
		} else {
			totalVSize += tx.Vsize
		}
	}

	if totalVSize == 0 {
		return nil
	}
	amt, err := btcutil.NewAmount(total - c.cfg.ChainID.DefaultCoinbase())
	if err != nil {
		return fmt.Errorf("fail to parse total block fee amount, err: %w", err)
	}

	feeRateSats := uint64(amt.ToUnit(btcutil.AmountSatoshi) / float64(totalVSize))
	if c.cfg.UTXO.DefaultMinRelayFeeSats > feeRateSats {
		feeRateSats = c.cfg.UTXO.DefaultMinRelayFeeSats
	}

	transactionSize := c.cfg.UTXO.EstimatedAverageTxSize

	if c.GetChain() == common.ZECChain {
		tx := wire.MsgTx{TxIn: make([]*wire.TxIn, c.getMaximumUtxosToSpend()-1)}
		memo := strings.Repeat("X", 80)
		feeRateSats = c.getGasCoinZEC(&tx, memo).Amount.Uint64()
		transactionSize = 1
	}

	resolution := uint64(c.cfg.BlockScanner.GasPriceResolution)
	feeRateSats = ((feeRateSats / resolution) + 1) * resolution

	c.networkFeeLock.Lock()
	defer c.networkFeeLock.Unlock()

	lastFeeRate := c.lastFeeRate.Load()
	feeDelta := new(big.Int).Sub(big.NewInt(int64(feeRateSats)), big.NewInt(int64(lastFeeRate)))
	feeDelta.Abs(feeDelta)
	if lastFeeRate != 0 && feeDelta.Cmp(big.NewInt(c.cfg.BlockScanner.GasPriceResolution)) != 1 {
		return nil
	}

	c.log.Info().
		Int64("height", height).
		Uint64("lastFeeRate", lastFeeRate).
		Uint64("feeRateSats", feeRateSats).
		Msg("sendNetworkFee")

	c.globalNetworkFeeQueue <- common.NetworkFee{
		Chain:           c.cfg.ChainID,
		Height:          height,
		TransactionSize: transactionSize,
		TransactionRate: feeRateSats,
	}

	c.lastFeeRate.Store(feeRateSats)

	return nil
}

func (c *Client) getBlock(height int64) (*btcjson.GetBlockVerboseTxResult, error) {
	hash, err := c.rpc.GetBlockHash(height)
	if err != nil {
		return &btcjson.GetBlockVerboseTxResult{}, err
	}
	return c.rpc.GetBlockVerboseTxs(hash)
}

func (c *Client) isValidUTXO(hexPubKey string) bool {
	buf, decErr := hex.DecodeString(hexPubKey)
	if decErr != nil {
		c.log.Err(decErr).Msgf("fail to decode hex string, %s", hexPubKey)
		return false
	}

	switch c.cfg.ChainID {
	case common.BTCChain, common.ZECChain:
		scriptType, addresses, requireSigs, err := btctxscript.ExtractPkScriptAddrs(buf, c.getChainCfgBTC())
		if err != nil {
			c.log.Err(err).Msg("fail to extract pub key script")
			return false
		}
		switch scriptType {
		case btctxscript.MultiSigTy:
			return false
		default:
			return len(addresses) == 1 && requireSigs == 1
		}

	default:
		c.log.Fatal().Msg("unsupported chain")
		return false
	}
}

func (c *Client) isRBFEnabled(tx *btcjson.TxRawResult) bool {
	for _, vin := range tx.Vin {
		if vin.Sequence < (0xffffffff - 1) {
			return true
		}
	}
	return false
}

func (c *Client) getTxIn(tx *btcjson.TxRawResult, height int64, isMemPool bool, vinZeroTxs map[string]*btcjson.TxRawResult) (types.TxInItem, error) {
	if c.ignoreTx(tx, height) {
		c.log.Debug().Int64("height", height).Str("txid", tx.Hash).Msg("ignore tx not matching format")
		return types.TxInItem{}, nil
	}
	if c.isRBFEnabled(tx) && isMemPool {
		return types.TxInItem{}, nil
	}
	sender, err := c.getSender(tx, vinZeroTxs)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to get sender from tx: %w", err)
	}
	memo, err := c.getMemo(tx)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to get memo from tx: %w", err)
	}
	if len([]byte(memo)) > constants.MaxMemoSize {
		return types.TxInItem{}, fmt.Errorf("memo (%s) longer than max allow length (%d)", memo, constants.MaxMemoSize)
	}
	m, err := mem.ParseMemo(common.LatestVersion, memo)
	if err != nil {
		c.log.Debug().Err(err).Str("memo", memo).Msg("fail to parse memo")
	}
	output, err := c.getOutput(sender, tx, m.IsType(mem.TxConsolidate))
	if err != nil {
		if errors.Is(err, btypes.ErrFailOutputMatchCriteria) {
			c.log.Debug().Int64("height", height).Str("txid", tx.Hash).Msg("ignore tx not matching format")
			return types.TxInItem{}, nil
		}
		return types.TxInItem{}, fmt.Errorf("fail to get output from tx: %w", err)
	}

	addresses := c.getAddressesFromScriptPubKey(output.ScriptPubKey)
	toAddr := addresses[0]

	isInbound := c.isAsgardAddress(toAddr)
	if isInbound {
		if !c.isValidUTXO(output.ScriptPubKey.Hex) {
			return types.TxInItem{}, fmt.Errorf("invalid utxo")
		}
	}
	amount, err := btcutil.NewAmount(output.Value)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to parse float64: %w", err)
	}
	amt := uint64(amount.ToUnit(btcutil.AmountSatoshi))

	gas, err := c.getGas(tx, isInbound)
	if err != nil {
		return types.TxInItem{}, fmt.Errorf("fail to get gas from tx: %w", err)
	}
	return types.TxInItem{
		BlockHeight: height,
		Tx:          tx.Txid,
		Sender:      sender,
		To:          toAddr,
		Coins: common.Coins{
			common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(amt)),
		},
		Memo: memo,
		Gas:  gas,
	}, nil
}

func (c *Client) getVinZeroTxs(block *btcjson.GetBlockVerboseTxResult) (map[string]*btcjson.TxRawResult, error) {
	vinZeroTxs := make(map[string]*btcjson.TxRawResult)
	start := time.Now()

	dustThreshold := c.cfg.ChainID.DustThreshold().Uint64()

	batches := [][]string{}
	batch := []string{}
	var count, ignoreCount, failMemoSkipCount, skipDustCount int
	for i := range block.Tx {
		if c.ignoreTx(&block.Tx[i], block.Height) {
			ignoreCount++
			continue
		}

		voutSats, err := sumVoutSats(&block.Tx[i])
		if err != nil {
			c.log.Error().Err(err).Str("txid", block.Tx[i].Txid).Msg("fail to sum vout sats")
		} else if voutSats < dustThreshold {
			skipDustCount++
			continue
		}

		memo, err := c.getMemo(&block.Tx[i])
		if err != nil || len(memo) > constants.MaxMemoSize {
			failMemoSkipCount++
			continue
		}

		count++
		batch = append(batch, block.Tx[i].Vin[0].Txid)
		if len(batch) >= c.cfg.UTXO.TransactionBatchSize {
			batches = append(batches, batch)
			batch = []string{}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	c.log.Debug().
		Int64("height", block.Height).
		Int("ignoreCount", ignoreCount).
		Int("failMemoSkipCount", failMemoSkipCount).
		Int("skipDustCount", skipDustCount).
		Int("count", count).
		Int("batchSize", c.cfg.UTXO.TransactionBatchSize).
		Int("batchCount", len(batches)).
		Msg("getVinZeroTxs")

	retries := 0
	for i := 0; i < len(batches); i++ {
		results, errs, err := c.rpc.BatchGetRawTransactionVerbose(batches[i])

		txErrCount := 0
		if err == nil {
			for _, txErr := range errs {
				if txErr != nil {
					err = txErr
				}
				txErrCount++
			}
		}

		if err != nil {
			if retries >= 3 {
				return nil, err
			}

			c.log.Err(err).Int("txErrCount", txErrCount).Msgf("retrying block txs batch %d", i)
			time.Sleep(time.Second)
			retries++
			i--
			continue
		}

		for _, tx := range results {
			vinZeroTxs[tx.Txid] = tx
		}
	}

	c.log.Debug().
		Int64("height", block.Height).
		Dur("duration", time.Since(start)).
		Msg("getVinZeroTxs complete")

	return vinZeroTxs, nil
}

func (c *Client) extractTxs(block *btcjson.GetBlockVerboseTxResult) (types.TxIn, error) {
	txIn := types.TxIn{
		Chain:   c.GetChain(),
		MemPool: false,
	}

	var vinZeroTxs map[string]*btcjson.TxRawResult
	var err error
	if !c.disableVinZeroBatch {
		vinZeroTxs, err = c.getVinZeroTxs(block)
		if err != nil {
			c.log.Error().Err(err).Msg("fail to get txid to vin zero tx, getTxIn will fan out")
		}
	}

	var txItems []*types.TxInItem
	for idx, tx := range block.Tx {
		c.removeFromMemPoolCache(tx.Hash)
		var txInItem types.TxInItem
		txInItem, err = c.getTxIn(&block.Tx[idx], block.Height, false, vinZeroTxs)
		if err != nil {
			c.log.Debug().Str("txid", tx.Txid).Err(err).Msg("fail to get TxInItem")
			continue
		}
		if txInItem.IsEmpty() {
			continue
		}
		if txInItem.Coins.IsEmpty() {
			continue
		}
		if txInItem.Coins[0].Amount.LT(c.cfg.ChainID.DustThreshold()) {
			continue
		}
		var added bool
		added, err = c.temporalStorage.TrackObservedTx(txInItem.Tx)
		if err != nil {
			c.log.Err(err).Msgf("fail to determine whether hash(%s) had been observed before", txInItem.Tx)
		}
		if !added {
			c.log.Info().Msgf("tx: %s had been report before, ignore", txInItem.Tx)
			continue
		}
		txItems = append(txItems, &txInItem)
	}
	txIn.TxArray = txItems
	return txIn, nil
}

func (c *Client) ignoreTx(tx *btcjson.TxRawResult, height int64) bool {
	if len(tx.Vin) == 0 || len(tx.Vout) == 0 || len(tx.Vout) > 12 {
		return true
	}
	if tx.Vin[0].Txid == "" {
		return true
	}
	if tx.LockTime > uint32(height) {
		return true
	}
	countWithOutput := 0
	for _, vout := range tx.Vout {
		// analyze-ignore(float-comparison)
		if vout.Value > 0 {
			countWithOutput++
		}
	}

	if countWithOutput == 0 {
		return true
	}
	if countWithOutput > 10 {
		return true
	}
	return false
}

func (c *Client) getOutput(sender string, tx *btcjson.TxRawResult, consolidate bool) (btcjson.Vout, error) {
	isSenderAsgard := c.isAsgardAddress(sender)
	for _, vout := range tx.Vout {
		if strings.EqualFold(vout.ScriptPubKey.Type, "nulldata") {
			continue
		}
		// analyze-ignore(float-comparison)
		if vout.Value <= 0 {
			continue
		}
		addresses := c.getAddressesFromScriptPubKey(vout.ScriptPubKey)
		if len(addresses) != 1 {
			continue
		}
		receiver := addresses[0]
		if !isSenderAsgard && !c.isAsgardAddress(receiver) {
			continue
		}

		if consolidate && receiver == sender {
			return vout, nil
		}
		if !consolidate && receiver != sender {
			return vout, nil
		}
	}
	return btcjson.Vout{}, btypes.ErrFailOutputMatchCriteria
}

func (c *Client) isFromAsgard(txid string) bool {
	tx, err := c.rpc.GetRawTransactionVerbose(txid)
	if err != nil {
		c.log.Error().Err(err).Str("txid", txid).Msg("fail to get tx")
		return false
	}

	sender, err := c.getSender(tx, nil)
	if err != nil {
		c.log.Error().Err(err).Str("txid", txid).Msg("fail to get sender")
		return false
	}

	return c.isAsgardAddress(sender)
}

func (c *Client) getSender(tx *btcjson.TxRawResult, vinZeroTxs map[string]*btcjson.TxRawResult) (string, error) {
	if len(tx.Vin) == 0 {
		return "", fmt.Errorf("no vin available in tx")
	}

	var vout btcjson.Vout
	if vinZeroTxs != nil {
		vinTx, ok := vinZeroTxs[tx.Vin[0].Txid]
		if !ok {
			value, err := sumVoutSats(tx)
			if err != nil || value >= c.cfg.ChainID.DustThreshold().Uint64() {
				c.log.Debug().Str("txid", tx.Txid).Msg("vin zero tx not found")
			}
			return "", fmt.Errorf("missing vin zero tx")
		}
		vout = vinTx.Vout[tx.Vin[0].Vout]
	} else {
		vinTx, err := c.rpc.GetRawTransactionVerbose(tx.Vin[0].Txid)
		if err != nil {
			return "", fmt.Errorf("fail to query raw tx")
		}
		vout = vinTx.Vout[tx.Vin[0].Vout]
	}

	addresses := c.getAddressesFromScriptPubKey(vout.ScriptPubKey)
	if len(addresses) == 0 {
		return "", fmt.Errorf("no address available in vout")
	}
	address := addresses[0]

	return address, nil
}

func (c *Client) getAddressesFromScriptPubKey(scriptPubKey btcjson.ScriptPubKeyResult) []string {
	if c.cfg.ChainID.Equals(common.BTCChain) {
		return c.getAddressesFromScriptPubKeyBTC(scriptPubKey)
	}
	return scriptPubKey.Addresses
}

func (c *Client) getMemo(tx *btcjson.TxRawResult) (string, error) {
	var memo string

	if c.cfg.ChainID.Equals(common.ZECChain) {
		if len(tx.Vin) == 0 || len(tx.Vout) == 0 {
			c.log.Error().Str("txid", tx.Txid).Msg("shielded tx")
			return "", nil
		}
	}

	for _, vOut := range tx.Vout {
		switch strings.ToLower(vOut.ScriptPubKey.Type) {
		case "witness_v0_keyhash", "pubkeyhash", "nulldata":
		default:
			continue
		}

		buf, err := hex.DecodeString(vOut.ScriptPubKey.Hex)
		if err != nil {
			c.log.Err(err).Msg("fail to hex decode scriptPubKey")
			continue
		}

		var asm string
		switch c.cfg.ChainID {
		case common.BTCChain, common.ZECChain:
			asm, err = btctxscript.DisasmString(buf)
		default:
			c.log.Fatal().Msg("unsupported chain")
		}

		if err != nil {
			c.log.Err(err).Msg("fail to disasm script pubkey")
			continue
		}
		fields := strings.Fields(asm)

		if len(fields) < 2 {
			continue
		}

		if fields[0] == "OP_RETURN" {
			if fields[1] == "0" {
				continue
			}

			var decoded string
			decoded, err = c.decodeHexString(fields[1])
			if err != nil {
				return "", nil
			}
			memo += decoded
			continue
		}

		if len(memo) < constants.MaxOpReturnDataSize {
			continue
		}

		if strings.LastIndex(memo, "^") < constants.MaxOpReturnDataSize-1 {
			continue
		}

		var pubkey string

		switch len(fields) {
		case 2:
			if fields[0] != "0" {
				continue
			}
			pubkey = fields[1]
		case 5:
			requiredOps := []string{
				"OP_DUP", "OP_HASH160", fields[2],
				"OP_EQUALVERIFY", "OP_CHECKSIG",
			}

			isValidScript := true
			for i := 0; i < 4; i++ {
				if fields[i] != requiredOps[i] {
					isValidScript = false
					break
				}
			}

			if !isValidScript {
				continue
			}

			pubkey = fields[2]
		default:
			continue
		}

		if len(pubkey) != 40 {
			continue
		}

		pubkey = c.regexpRemoveTrailingZeros.ReplaceAllString(pubkey, "")

		decoded, err := c.decodeHexString(pubkey)
		if err != nil {
			return "", nil
		}
		memo += decoded

		if len(pubkey) != 40 {
			break
		}

		if len(memo) >= constants.MaxMemoSize {
			break
		}
	}

	if strings.LastIndex(memo, "^") >= constants.MaxOpReturnDataSize-1 {
		memo = strings.Replace(memo, "^", "", 1)
	}

	return memo, nil
}

func (c *Client) decodeHexString(hexString string) (string, error) {
	errMsg := "fail to decode data: " + hexString

	decoded, err := hex.DecodeString(hexString)
	if err != nil {
		c.log.Debug().Err(err).Msg(errMsg)
		return "", err
	}

	for i, b := range decoded {
		if b < 0x20 || b > 0x7E {
			err := fmt.Errorf("invalid hex value at position %d: 0x%X", i, b)
			c.log.Debug().Err(err).Msg(errMsg)
			return "", err
		}
	}

	return string(decoded), nil
}

func (c *Client) getGas(tx *btcjson.TxRawResult, isInbound bool) (common.Gas, error) {
	var sumVin uint64 = 0
	for _, vin := range tx.Vin {
		vinTx, err := c.rpc.GetRawTransactionVerbose(vin.Txid)
		if err != nil {
			return common.Gas{}, fmt.Errorf("fail to query raw tx from node")
		}

		amount, err := btcutil.NewAmount(vinTx.Vout[vin.Vout].Value)
		if err != nil {
			return nil, err
		}
		sumVin += uint64(amount.ToUnit(btcutil.AmountSatoshi))
	}
	var sumVout uint64 = 0
	for _, vout := range tx.Vout {
		if !isInbound && strings.ToLower(vout.ScriptPubKey.Type) == "nulldata" {
			break
		}

		amount, err := btcutil.NewAmount(vout.Value)
		if err != nil {
			return nil, err
		}
		sumVout += uint64(amount.ToUnit(btcutil.AmountSatoshi))
	}
	totalGas := sumVin - sumVout
	return common.Gas{
		common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(totalGas)),
	}, nil
}

func (c *Client) getCoinbaseValue(blockHeight int64) (int64, error) {
	result, err := c.getBlock(blockHeight)
	if err != nil {
		return 0, fmt.Errorf("fail to get block verbose tx: %w", err)
	}
	for _, tx := range result.Tx {
		if len(tx.Vin) == 1 && tx.Vin[0].IsCoinBase() {
			total := float64(0)
			for _, opt := range tx.Vout {
				total += opt.Value
			}
			var amt btcutil.Amount
			amt, err = btcutil.NewAmount(total)
			if err != nil {
				return 0, fmt.Errorf("fail to parse amount: %w", err)
			}
			return int64(amt), nil
		}
	}
	return 0, fmt.Errorf("fail to get coinbase value")
}

func (c *Client) getBlockRequiredConfirmation(txIn types.TxIn, height int64) (int64, error) {
	asgardAddresses, err := c.getAsgardAddress()
	if err != nil {
		c.log.Err(err).Msg("fail to get asgard addresses")
	}
	totalTxValue := txIn.GetTotalTransactionValue(c.cfg.ChainID.GetGasAsset(), asgardAddresses)
	totalFeeAndSubsidy, err := c.getCoinbaseValue(height)
	if err != nil {
		c.log.Err(err).Msgf("fail to get coinbase value")
	}
	confMul, err := utxo.GetConfMulBasisPoint(c.GetChain().String(), c.bridge)
	if err != nil {
		c.log.Err(err).Msgf("fail to get conf multiplier mimir value for %s", c.GetChain().String())
	}
	if totalFeeAndSubsidy == 0 {
		var cbValue btcutil.Amount
		cbValue, err = btcutil.NewAmount(c.cfg.ChainID.DefaultCoinbase())
		if err != nil {
			return 0, fmt.Errorf("fail to get default coinbase value: %w", err)
		}
		totalFeeAndSubsidy = int64(cbValue)
	}
	confValue := common.GetUncappedShare(confMul, cosmos.NewUint(constants.MaxBasisPts), cosmos.SafeUintFromInt64(totalFeeAndSubsidy))
	confirm := totalTxValue.Quo(confValue).Uint64()
	confirm, err = utxo.MaxConfAdjustment(confirm, c.GetChain().String(), c.bridge)
	if err != nil {
		c.log.Err(err).Msgf("fail to get max conf value adjustment for %s", c.GetChain().String())
	}
	if confirm < c.cfg.MinConfirmations {
		confirm = c.cfg.MinConfirmations
	}
	c.log.Info().Msgf("totalTxValue:%s, totalFeeAndSubsidy:%d, confirm:%d", totalTxValue, totalFeeAndSubsidy, confirm)

	return int64(confirm), nil
}
