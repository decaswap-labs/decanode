package utxo

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/utxo/zecutil"

	"github.com/btcsuite/btcutil"
	btctxscript "github.com/decaswap-labs/decanode/bifrost/txscript/txscript"

	stypes "github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	mem "github.com/decaswap-labs/decanode/x/thorchain/memo"
	"github.com/decaswap-labs/decanode/x/thorchain/types"

	"github.com/decaswap-labs/decanode/constants"
)

////////////////////////////////////////////////////////////////////////////////////////
// UTXO Selection
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) getMaximumUtxosToSpend() int64 {
	const mimirMaxUTXOsToSpend = `MaxUTXOsToSpend`
	utxosToSpend, err := c.bridge.GetMimir(mimirMaxUTXOsToSpend)
	if err != nil {
		c.log.Err(err).Msg("fail to get MaxUTXOsToSpend")
	}
	if utxosToSpend <= 0 {
		utxosToSpend = c.cfg.UTXO.MaxUTXOsToSpend
	}
	return utxosToSpend
}

func (c *Client) getUtxoToSpend(pubkey common.PubKey, total btcutil.Amount, sweepDust bool) ([]btcjson.ListUnspentResult, error) {
	addr, err := pubkey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("fail to get address from pubkey(%s): %w", pubkey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return nil, fmt.Errorf("fail to get UTXOs: %w", err)
	}

	sort.SliceStable(utxos, func(i, j int) bool {
		if utxos[i].Confirmations > utxos[j].Confirmations {
			return true
		} else if utxos[i].Confirmations < utxos[j].Confirmations {
			return false
		}
		return utxos[i].TxID < utxos[j].TxID
	})

	var result []btcjson.ListUnspentResult
	var toSpend btcutil.Amount
	minUTXOAmt := btcutil.Amount(c.cfg.ChainID.DustThreshold().Uint64()).ToBTC()
	utxosToSpend := c.getMaximumUtxosToSpend()

	for _, item := range utxos {
		if !c.isValidUTXO(item.ScriptPubKey) {
			c.log.Warn().Str("script", item.ScriptPubKey).Msgf("invalid utxo, unable to spend")
			continue
		}

		// analyze-ignore(float-comparison)
		if item.Confirmations < c.cfg.UTXO.MinUTXOConfirmations || item.Amount < minUTXOAmt {
			if sweepDust && item.Amount < minUTXOAmt && item.Confirmations >= c.cfg.UTXO.MinUTXOConfirmations {
			} else {
				isSelfTx := c.isSelfTransaction(item.TxID)

				if !isSelfTx {
					isSelfTx = c.isFromAsgard(item.TxID)
				}
				if !isSelfTx {
					continue
				}
			}

			if item.Confirmations == 0 && c.cfg.UTXO.MaxMempoolAncestors > 0 {
				var entry *btcjson.GetMempoolEntryResult
				entry, err = c.rpc.GetMempoolEntry(item.TxID)
				if err != nil {
					c.log.Debug().Err(err).Str("txid", item.TxID).Msg("failed to get mempool entry")
				}

				if err == nil && entry.AncestorCount+entry.DescendantCount >= c.cfg.UTXO.MaxMempoolAncestors {
					c.log.Warn().
						Str("txid", item.TxID).
						Int64("ancestor_count", entry.AncestorCount).
						Int64("descendant_count", entry.DescendantCount).
						Int64("max_allowed", c.cfg.UTXO.MaxMempoolAncestors).
						Msg("skipping UTXO with too many ancestors/descendants to avoid mempool chain limit")
					continue
				}
			}
		}

		if c.cfg.ChainID == common.ZECChain {
			id := formatUtxoKey(item.TxID, item.Vout)

			var found bool
			found, err = c.temporalStorage.HasSpentUtxo(id)
			if err != nil {
				c.log.Err(err).Msg("failed to check spent utxo")
				continue
			}

			if found {
				continue
			}
		}

		result = append(result, item)
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("fail to convert to btcutil amount: %w", err)
		}
		toSpend += amt

		if int64(len(result)) >= utxosToSpend && toSpend >= total {
			break
		}
	}

	if toSpend < total {
		return nil, fmt.Errorf("insufficient available UTXOs: need %d, only have %d available from %d UTXOs", total, toSpend, len(result))
	}

	return result, nil
}

func formatUtxoKey(txID string, vout uint32) string {
	return fmt.Sprintf("%s-%d", txID, vout)
}

func (c *Client) vinsUnspent(tx stypes.TxOutItem, vins []*wire.TxIn) (bool, error) {
	addr, err := tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return false, fmt.Errorf("fail to get address from pubkey(%s): %w", tx.VaultPubKey, err)
	}
	utxos, err := c.rpc.ListUnspent(addr.String())
	if err != nil {
		return false, fmt.Errorf("fail to get UTXOs: %w", err)
	}
	unspent := make(map[string]bool, len(utxos))
	for _, utxo := range utxos {
		unspent[formatUtxoKey(utxo.TxID, utxo.Vout)] = true
	}

	allUnspent := true
	for _, vin := range vins {
		key := formatUtxoKey(vin.PreviousOutPoint.Hash.String(), vin.PreviousOutPoint.Index)
		if c.cfg.ChainID == common.ZECChain {
			var found bool
			found, err = c.temporalStorage.HasSpentUtxo(key)
			if err != nil {
				return false, fmt.Errorf("fail to check spent utxo(%s): %w", key, err)
			}

			if found {
				c.log.Warn().
					Stringer("in_hash", tx.InHash).
					Str("txid", vin.PreviousOutPoint.Hash.String()).
					Uint32("vout", vin.PreviousOutPoint.Index).
					Msg("vin is marked spent in local cache")
				allUnspent = false
				continue
			}
		}

		if !unspent[key] {
			c.log.Warn().
				Stringer("in_hash", tx.InHash).
				Str("txid", vin.PreviousOutPoint.Hash.String()).
				Uint32("vout", vin.PreviousOutPoint.Index).
				Msg("vin is spent")
			allUnspent = false
		}
	}

	return allUnspent, nil
}

func (c *Client) isSelfTransaction(txID string) bool {
	bms, err := c.temporalStorage.GetBlockMetas()
	if err != nil {
		c.log.Err(err).Msg("fail to get block metas")
		return false
	}
	for _, item := range bms {
		for _, tx := range item.SelfTransactions {
			if strings.EqualFold(tx, txID) {
				c.log.Debug().Msgf("%s is self transaction", txID)
				return true
			}
		}
	}
	return false
}

func (c *Client) getPaymentAmount(tx stypes.TxOutItem) btcutil.Amount {
	amtToPay := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset()).Amount.Uint64()
	if !tx.MaxGas.IsEmpty() && c.cfg.ChainID != common.ZECChain {
		gasAmt := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset()).Amount
		amtToPay += gasAmt.Uint64()
	}
	return btcutil.Amount(amtToPay)
}

func (c *Client) getSourceScript(tx stypes.TxOutItem) ([]byte, error) {
	sourceAddr, err := tx.VaultPubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("fail to get source address: %w", err)
	}

	switch c.cfg.ChainID {
	case common.BTCChain:
		var addr btcutil.Address
		addr, err = btcutil.DecodeAddress(sourceAddr.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return btctxscript.PayToAddrScript(addr)
	case common.ZECChain:
		params := c.getChainCfgZEC()
		addr, err := zecutil.DecodeAddress(sourceAddr.String(), params.Name)
		if err != nil {
			return nil, fmt.Errorf("fail to decode source address(%s): %w", sourceAddr.String(), err)
		}
		return zecutil.PayToAddrScript(addr)
	default:
		return nil, fmt.Errorf("unsupported chain: %s", c.cfg.ChainID)
	}
}

func (c *Client) estimateTxSize(txes []btcjson.ListUnspentResult, memoScripts [][]byte, customerScript, changeScript []byte) int64 {
	tx := wire.NewMsgTx(wire.TxVersion)

	for _, utxo := range txes {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			c.log.Error().Err(err).Msg("failed to parse txid for size estimation")
			continue
		}
		outpoint := wire.NewOutPoint(hash, utxo.Vout)
		txIn := wire.NewTxIn(outpoint, nil, nil)

		if c.isSegwitChain() {
			txIn.Witness = make([][]byte, 2)
			txIn.Witness[0] = make([]byte, 72)
			txIn.Witness[1] = make([]byte, 33)
		} else {
			txIn.SignatureScript = make([]byte, 107)
		}

		tx.AddTxIn(txIn)
	}

	tx.AddTxOut(wire.NewTxOut(0, customerScript))
	tx.AddTxOut(wire.NewTxOut(0, changeScript))

	if len(memoScripts) > 0 {
		tx.AddTxOut(wire.NewTxOut(0, memoScripts[0]))
		for _, script := range memoScripts[1:] {
			tx.AddTxOut(wire.NewTxOut(0, script))
		}
	}

	if c.isSegwitChain() {
		strippedSize := tx.SerializeSizeStripped()
		totalSize := tx.SerializeSize()
		return int64((strippedSize*3 + totalSize + 3) / 4)
	}

	return int64(tx.SerializeSize())
}

func (c *Client) isSegwitChain() bool {
	switch c.cfg.ChainID {
	case common.BTCChain:
		return true
	case common.ZECChain:
		return false
	default:
		c.log.Fatal().Msgf("unsupported chain: %s", c.cfg.ChainID)
		return false
	}
}

func (c *Client) getGasCoin(tx stypes.TxOutItem, vSize int64) common.Coin {
	gasRate := tx.GasRate

	if gasRate == 0 {
		fee, vBytes, err := c.temporalStorage.GetTransactionFee()
		if err != nil {
			c.log.Error().Err(err).Msg("fail to get previous transaction fee from local storage")
			return common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(vSize*gasRate)))
		}
		// analyze-ignore(float-comparison)
		if fee != 0.0 && vSize != 0 {
			var amt btcutil.Amount
			amt, err = btcutil.NewAmount(fee)
			if err != nil {
				c.log.Err(err).Msg("fail to convert amount from float64 to int64")
			} else {
				gasRate = int64(amt) / int64(vBytes)
			}
		}
	}

	if gasRate == 0 {
		gasRate = c.cfg.UTXO.DefaultSatsPerVByte
	}

	return common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(gasRate*vSize)))
}

func (c *Client) getGasCoinZEC(tx *wire.MsgTx, memo string) common.Coin {
	bytesOpReturn := 12 + len(memo)
	actionsMemo := (bytesOpReturn + 34 - 1) / 34

	amount := 5000*max(len(tx.TxIn), 2+actionsMemo) + ZecExtraFee

	return common.NewCoin(common.ZECAsset, cosmos.NewUint(uint64(amount)))
}

func (c *Client) buildTx(tx stypes.TxOutItem, sourceScript []byte) (*wire.MsgTx, map[string]int64, error) {
	isMigrate := false
	if memoStr := tx.GetMemo(); memoStr != "" {
		parsedMemo, mErr := mem.ParseMemo(common.LatestVersion, memoStr)
		if mErr == nil {
			isMigrate = parsedMemo.GetType() == mem.TxMigrate
		}
	}

	txes, err := c.getUtxoToSpend(tx.VaultPubKey, c.getPaymentAmount(tx), isMigrate)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get unspent UTXO: %w", err)
	}
	redeemTx := wire.NewMsgTx(wire.TxVersion)
	totalAmt := int64(0)
	individualAmounts := make(map[string]int64, len(txes))
	for _, item := range txes {
		var txID *chainhash.Hash
		txID, err = chainhash.NewHashFromStr(item.TxID)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse txID(%s): %w", item.TxID, err)
		}
		outputPoint := wire.NewOutPoint(txID, item.Vout)
		sourceTxIn := wire.NewTxIn(outputPoint, nil, nil)
		redeemTx.AddTxIn(sourceTxIn)
		var amt btcutil.Amount
		amt, err = btcutil.NewAmount(item.Amount)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse amount(%f): %w", item.Amount, err)
		}
		individualAmounts[formatUtxoKey(txID.String(), item.Vout)] = int64(amt)
		totalAmt += int64(amt)
	}

	var buf []byte
	var nullDataScripts [][]byte
	switch c.cfg.ChainID {
	case common.BTCChain:
		var outputAddr btcutil.Address
		outputAddr, err = btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = btctxscript.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, btctxscript.MaxDataCarrierSize, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	case common.ZECChain:
		var outputAddr btcutil.Address
		outputAddr, err = zecutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgZEC().Name)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		buf, err = zecutil.PayToAddrScript(outputAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to get pay to address script: %w", err)
		}
		nullDataScripts, err = MemoToScripts(tx.Memo, btctxscript.MaxDataCarrierSize, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to generate null data script: %w", err)
		}
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	if len(nullDataScripts) == 0 && len(tx.Memo) != 0 {
		return nil, nil, fmt.Errorf("no null data scripts generated, memo will not be included in the transaction")
	}

	memoForParsing := tx.GetMemo()
	var memo mem.Memo
	if memoForParsing == "" {
		memo = mem.NewOutboundMemo(tx.InHash)
	} else {
		memo, err = mem.ParseMemo(common.LatestVersion, memoForParsing)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to parse memo: %w", err)
		}
	}

	totalSize := c.estimateTxSize(txes, nullDataScripts, buf, sourceScript)

	coinToCustomer := tx.Coins.GetCoin(c.cfg.ChainID.GetGasAsset())

	var gasCoin common.Coin
	switch c.cfg.ChainID {
	case common.ZECChain:
		gasCoin = c.getGasCoinZEC(redeemTx, memo.String())
	default:
		gasCoin = c.getGasCoin(tx, totalSize)
	}

	maxFeeSats := totalSize * c.cfg.UTXO.MaxSatsPerVByte
	gasAmtSats := gasCoin.Amount.Uint64()

	if gasAmtSats > uint64(maxFeeSats) {
		diffSats := gasAmtSats - uint64(maxFeeSats)
		c.log.Info().Msgf("gas amount: %d is larger than maximum fee: %d, diff: %d", gasAmtSats, uint64(maxFeeSats), diffSats)
		gasAmtSats = uint64(maxFeeSats)
	} else {
		minRelayFeeSats := c.minRelayFeeSats.Load()
		if gasAmtSats < minRelayFeeSats {
			diffStats := minRelayFeeSats - gasAmtSats
			c.log.Info().Msgf("gas amount: %d is less than min relay fee: %d, diff remove from customer: %d", gasAmtSats, minRelayFeeSats, diffStats)
			gasAmtSats = minRelayFeeSats
		}
	}

	if !tx.MaxGas.IsEmpty() {
		maxGasCoin := tx.MaxGas.ToCoins().GetCoin(c.cfg.ChainID.GetGasAsset())
		if gasAmtSats > maxGasCoin.Amount.Uint64() {
			c.log.Info().Msgf("max gas: %s, however estimated gas need %d", tx.MaxGas, gasAmtSats)
			gasAmtSats = maxGasCoin.Amount.Uint64()
		} else if gasAmtSats < maxGasCoin.Amount.Uint64() && memo.GetType() == mem.TxMigrate {
			gap := maxGasCoin.Amount.Uint64() - gasAmtSats
			c.log.Info().Msgf("max gas is: %s, however only: %d is required, gap: %d goes to the vault migrated to", tx.MaxGas, gasAmtSats, gap)
			coinToCustomer.Amount = coinToCustomer.Amount.Add(cosmos.NewUint(gap))
		}
	} else if memo.GetType() == mem.TxConsolidate {
		gap := gasAmtSats
		c.log.Info().Msgf("consolidate tx, need gas: %d", gap)
		coinToCustomer.Amount = common.SafeSub(coinToCustomer.Amount, cosmos.NewUint(gap))
	}

	gasAmt := btcutil.Amount(gasAmtSats)
	err = c.temporalStorage.UpsertTransactionFee(gasAmt.ToBTC(), int32(totalSize))
	if err != nil {
		c.log.Err(err).Msg("fail to save gas info to UTXO storage")
	}

	redeemTxOut := wire.NewTxOut(int64(coinToCustomer.Amount.Uint64()), buf)
	redeemTx.AddTxOut(redeemTxOut)

	p2wpkhOutputsCost := int64(0)
	if len(nullDataScripts) > 1 {
		p2wpkhOutputsCost = int64(len(nullDataScripts)-1) * tx.Chain.P2WPKHOutputValue()
	}

	balance := totalAmt - redeemTxOut.Value - int64(gasAmt) - p2wpkhOutputsCost
	c.log.Info().Msgf("total: %d, to customer: %d, gas: %d, p2wpkh_outputs_cost: %d", totalAmt, redeemTxOut.Value, int64(gasAmt), p2wpkhOutputsCost)
	if balance < 0 {
		return nil, nil, fmt.Errorf("not enough balance to pay customer: %d", balance)
	}
	if balance > 0 {
		c.log.Info().Msgf("send %d back to self", balance)
		redeemTx.AddTxOut(wire.NewTxOut(balance, sourceScript))
	}

	if len(tx.Memo) != 0 {
		redeemTx.AddTxOut(wire.NewTxOut(0, nullDataScripts[0]))
		for _, script := range nullDataScripts[1:] {
			redeemTx.AddTxOut(wire.NewTxOut(tx.Chain.P2WPKHOutputValue(), script))
		}
	}

	return redeemTx, individualAmounts, nil
}

func MemoToScripts(memo string, maxDataCarrierSize int, nullDataScript, payToWitnessKeyHashScript func([]byte) ([]byte, error)) ([][]byte, error) {
	if len(memo) == 0 {
		return nil, nil
	}

	if len(memo) > constants.MaxMemoSize {
		return nil, fmt.Errorf("memo size %d exceeds maximum size of %d bytes", len(memo), constants.MaxMemoSize)
	}

	data := []byte(memo)

	remainingDataSize := len(data)
	if remainingDataSize > maxDataCarrierSize {
		remainingDataSize -= (maxDataCarrierSize - 1)
	} else {
		remainingDataSize = 0
	}
	numScripts := 1 + (remainingDataSize+19)/20
	scripts := make([][]byte, 0, numScripts)

	firstChunkSize := len(data)
	continuation := false
	if firstChunkSize > maxDataCarrierSize {
		firstChunkSize = maxDataCarrierSize - 1
		continuation = true
	}
	firstChunk := make([]byte, 0, maxDataCarrierSize)
	firstChunk = append(firstChunk, data[:firstChunkSize]...)
	if continuation {
		firstChunk = append(firstChunk, '^')
	}
	script, err := nullDataScript(firstChunk)
	if err != nil {
		return nil, fmt.Errorf("fail to create OP_RETURN script: %w", err)
	}
	scripts = append(scripts, script)

	if continuation {
		remainingData := data[firstChunkSize:]
		for i := 0; len(remainingData) > 0; i++ {
			chunkSize := len(remainingData)
			if chunkSize > 20 {
				chunkSize = 20
			}
			hash := make([]byte, 20)
			copy(hash, remainingData[:chunkSize])
			p2wpkhScript, err := payToWitnessKeyHashScript(hash)
			if err != nil {
				return nil, fmt.Errorf("fail to create P2WPKH script at index %d: %w", i, err)
			}
			scripts = append(scripts, p2wpkhScript)
			remainingData = remainingData[chunkSize:]
		}
	}

	return scripts, nil
}

////////////////////////////////////////////////////////////////////////////////////////
// UTXO Consolidation
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) consolidateVaultUTXOs(vault types.Vault, utxosToSpend int64) error {
	lock := c.GetVaultLock(vault.PubKey.String())

	lock.Lock()
	defer lock.Unlock()

	utxos, err := c.getUtxoToSpend(vault.PubKey, 0, false)
	if err != nil {
		return fmt.Errorf("get utxos to spend: %w", err)
	}

	if int64(len(utxos)) < utxosToSpend {
		return nil
	}

	txOutItem, err := c.buildConsolidateTxOutItem(vault, utxos)
	if err != nil {
		return err
	}

	height, err := c.bridge.GetBlockHeight()
	if err != nil {
		return fmt.Errorf("get THORChain block height: %w", err)
	}

	rawTx, _, _, err := c.SignTx(txOutItem, height)
	if err != nil {
		return fmt.Errorf("sign consolidate txout item: %w", err)
	}
	if len(rawTx) == 0 {
		c.log.Warn().Str("vault_pubkey", vault.PubKey.String()).Msg("signed consolidate transaction is empty, skipping broadcast")
		return nil
	}

	txID, err := c.BroadcastTx(txOutItem, rawTx)
	if err != nil {
		return fmt.Errorf("broadcast consolidate tx: %w", err)
	}
	c.log.Info().Str("vault_pubkey", vault.PubKey.String()).Msgf("broadcast consolidate tx successfully, hash:%s", txID)
	return nil
}

func (c *Client) buildConsolidateTxOutItem(vault types.Vault, utxos []btcjson.ListUnspentResult) (stypes.TxOutItem, error) {
	total := btcutil.Amount(0)
	for _, item := range utxos {
		amt, err := btcutil.NewAmount(item.Amount)
		if err != nil {
			return stypes.TxOutItem{}, fmt.Errorf("convert utxo amount %f: %w", item.Amount, err)
		}
		total += amt
	}

	addr, err := vault.PubKey.GetAddress(c.cfg.ChainID)
	if err != nil {
		return stypes.TxOutItem{}, fmt.Errorf("get address for pubkey %s: %w", vault.PubKey, err)
	}

	feeRate := math.Ceil(float64(c.lastFeeRate.Load()) * 3 / 2)

	return stypes.TxOutItem{
		Chain:            c.cfg.ChainID,
		ToAddress:        addr,
		VaultPubKey:      vault.PubKey,
		VaultPubKeyEddsa: vault.PubKeyEddsa,
		Coins: common.Coins{
			common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(total))),
		},
		Memo:    mem.NewConsolidateMemo().String(),
		MaxGas:  nil,
		GasRate: int64(feeRate),
	}, nil
}

func (c *Client) consolidateUTXOs() {
	defer func() {
		c.wg.Done()
		c.consolidateInProgress.Store(false)
	}()

	nodeStatus, err := c.bridge.FetchNodeStatus()
	if err != nil {
		c.log.Err(err).Msg("fail to get node status")
		return
	}
	if nodeStatus != types.NodeStatus_Active {
		c.log.Info().Msgf("node is not active , doesn't need to consolidate utxos")
		return
	}
	vaults, err := c.bridge.GetAsgards()
	if err != nil {
		c.log.Err(err).Msg("fail to get current asgards")
		return
	}
	utxosToSpend := c.getMaximumUtxosToSpend()
	for _, vault := range vaults {
		if !vault.Contains(c.nodePubKey) {
			continue
		}
		err = c.consolidateVaultUTXOs(vault, utxosToSpend)
		if err != nil {
			c.log.Err(err).Str("vault_pubkey", vault.PubKey.String()).Msg("fail to consolidate utxos for vault")
		}
	}
}
