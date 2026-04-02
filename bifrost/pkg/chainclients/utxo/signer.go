package utxo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/utxo/zecutil"

	btcwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"

	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/shared/utxo"
	stypes "github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"

	"github.com/btcsuite/btcd/mempool"
	"github.com/hashicorp/go-multierror"
)

const ZecTxExpiry = uint32(40)

const ZecExtraFee = int(0)

////////////////////////////////////////////////////////////////////////////////////////
// Client - Signing
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) SignTx(tx stypes.TxOutItem, thorchainHeight int64) ([]byte, []byte, *stypes.TxInItem, error) {
	if !tx.Chain.Equals(c.cfg.ChainID) {
		return nil, nil, nil, errors.New("wrong chain")
	}

	if tx.Coins.IsEmpty() {
		return nil, nil, nil, nil
	}

	if c.signerCacheManager.HasSigned(tx.CacheHash()) {
		c.log.Info().Msgf("ignoring already signed transaction: (%+v)", tx)
		return nil, nil, nil, nil
	}

	sourceScript, err := c.getSourceScript(tx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to get source pay to address script: %w", err)
	}

	var outputAddr interface{}
	var outputAddrStr string
	switch c.cfg.ChainID {
	case common.BTCChain:
		outputAddr, err = btcutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgBTC())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = outputAddr.(btcutil.Address).String()
	case common.ZECChain:
		outputAddr, err = zecutil.DecodeAddress(tx.ToAddress.String(), c.getChainCfgZEC().Name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to decode next address: %w", err)
		}
		outputAddrStr = tx.ToAddress.String()
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	if !strings.EqualFold(outputAddrStr, tx.ToAddress.String()) {
		c.log.Info().Msgf("output address: %s, to address: %s can't roundtrip", outputAddrStr, tx.ToAddress.String())
		return nil, nil, nil, nil
	}
	switch outputAddr.(type) {
	case *btcutil.AddressPubKey:
		c.log.Info().Msgf("address: %s is address pubkey type, should not be used", outputAddrStr)
		return nil, nil, nil, nil
	default:
	}

	checkpoint := utxo.SignCheckpoint{}
	redeemTx := &btcwire.MsgTx{}
	if tx.Checkpoint != nil {
		if err = json.Unmarshal(tx.Checkpoint, &checkpoint); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to unmarshal checkpoint: %w", err)
		}
		if err = redeemTx.Deserialize(bytes.NewReader(checkpoint.UnsignedTx)); err != nil {
			return nil, nil, nil, fmt.Errorf("fail to deserialize tx: %w", err)
		}

		c.log.Info().Stringer("in_hash", tx.InHash).Msgf("verifying checkpoint vins")
		var unspent bool
		unspent, err = c.vinsUnspent(tx, redeemTx.TxIn)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to verify checkpoint vins: %w", err)
		}
		if !unspent {
			return nil, nil, nil, nil
		}

	} else {
		redeemTx, checkpoint.IndividualAmounts, err = c.buildTx(tx, sourceScript)
		if err != nil {
			return nil, nil, nil, err
		}
		buf := bytes.NewBuffer([]byte{})
		err = redeemTx.Serialize(buf)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to serialize tx: %w", err)
		}
		checkpoint.UnsignedTx = buf.Bytes()
	}

	var checkpointBytes []byte
	checkpointBytes, err = json.Marshal(checkpoint)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to marshal checkpoint: %w", err)
	}

	c.log.Info().Msgf("UTXOs to sign: %d", len(redeemTx.TxIn))
	signings := []struct{ idx, amount int64 }{}
	totalAmount := int64(0)
	for idx, txIn := range redeemTx.TxIn {
		key := formatUtxoKey(txIn.PreviousOutPoint.Hash.String(), txIn.PreviousOutPoint.Index)
		outputAmount := checkpoint.IndividualAmounts[key]
		totalAmount += outputAmount
		signings = append(signings, struct{ idx, amount int64 }{int64(idx), outputAmount})
	}

	var stx interface{}
	switch c.cfg.ChainID {
	case common.BTCChain:
		stx = wireToBTC(redeemTx)
	case common.ZECChain:
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	chainHeight, err := c.rpc.GetBlockCount()
	if err != nil {
		chainHeight = c.currentBlockHeight.Load()
		c.log.Warn().Err(err).
			Int64("fallback_height", chainHeight).
			Msg("failed to get block height from RPC, falling back to scanner height")
	}

	var zecTx *zecutil.MsgTx
	if c.cfg.ChainID == common.ZECChain {
		redeemTx.Version = 4

		zecTx = &zecutil.MsgTx{
			MsgTx:        redeemTx,
			ExpiryHeight: uint32(chainHeight) + ZecTxExpiry,
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(signings))
	mu := &sync.Mutex{}
	var utxoErr error
	for _, signing := range signings {
		go func(i int, amount int64) {
			defer wg.Done()

			// trunk-ignore(golangci-lint/govet): shadow
			var err error

			switch c.cfg.ChainID {
			case common.BTCChain:
				err = c.signUTXOBTC(stx.(*btcwire.MsgTx), tx, amount, sourceScript, i)
			case common.ZECChain:
				err = c.signUTXOZEC(zecTx, tx, amount, sourceScript, i)
			default:
				c.log.Fatal().Msg("unsupported chain")
			}

			if err != nil {
				mu.Lock()
				utxoErr = multierror.Append(utxoErr, err)
				mu.Unlock()
			}
		}(int(signing.idx), signing.amount)
	}
	wg.Wait()
	if utxoErr != nil {
		err = utxo.PostKeysignFailure(c.bridge, tx, c.log, thorchainHeight, utxoErr)
		return nil, checkpointBytes, nil, fmt.Errorf("fail to sign the message: %w", err)
	}

	switch c.cfg.ChainID {
	case common.BTCChain:
		redeemTx = btcToWire(stx.(*btcwire.MsgTx))
	case common.ZECChain:
	default:
		c.log.Fatal().Msg("unsupported chain")
	}

	var signedTx bytes.Buffer

	switch c.cfg.ChainID {
	case common.ZECChain:
		err = zecTx.ZecEncode(&signedTx, 0, btcwire.BaseEncoding)
	default:
		finalSize := redeemTx.SerializeSize()
		finalVBytes := mempool.GetTxVirtualSize(btcutil.NewTx(redeemTx))
		c.log.Info().Msgf("final size: %d, final vbyte: %d", finalSize, finalVBytes)

		err = redeemTx.Serialize(&signedTx)
	}

	if err != nil {
		return nil, nil, nil, fmt.Errorf("fail to serialize tx to bytes: %w", err)
	}

	amt := redeemTx.TxOut[0].Value
	gas := totalAmount
	for _, txOut := range redeemTx.TxOut {
		gas -= txOut.Value
	}
	var txIn *stypes.TxInItem
	sender, err := tx.VaultPubKey.GetAddress(tx.Chain)
	if err == nil {
		txIn = stypes.NewTxInItem(
			chainHeight,
			redeemTx.TxHash().String(),
			tx.Memo,
			sender.String(),
			tx.ToAddress.String(),
			common.NewCoins(
				common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(amt))),
			),
			common.Gas(common.NewCoins(
				common.NewCoin(c.cfg.ChainID.GetGasAsset(), cosmos.NewUint(uint64(gas))),
			)),
			tx.VaultPubKey,
			"",
			"",
			nil,
		)
	}

	if c.cfg.ChainID == common.ZECChain {
		var ids []string
		for _, vin := range redeemTx.TxIn {
			id := formatUtxoKey(vin.PreviousOutPoint.Hash.String(), vin.PreviousOutPoint.Index)
			ids = append(ids, id)
		}
		err = c.temporalStorage.SetSpentUtxos(ids, chainHeight)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fail to set utxos: %w", err)
		}
	}

	return signedTx.Bytes(), nil, txIn, nil
}

func (c *Client) GetVaultLock(vaultPubKey string) *sync.Mutex {
	c.signerLock.Lock()
	defer c.signerLock.Unlock()
	l, ok := c.vaultLocks[vaultPubKey]
	if !ok {
		newLock := &sync.Mutex{}
		c.vaultLocks[vaultPubKey] = newLock
		return newLock
	}
	return l
}

////////////////////////////////////////////////////////////////////////////////////////
// Client - Broadcast
////////////////////////////////////////////////////////////////////////////////////////

func (c *Client) BroadcastTx(txOut stypes.TxOutItem, payload []byte) (string, error) {
	height, err := c.rpc.GetBlockCount()
	if err != nil {
		return "", fmt.Errorf("fail to get block height: %w", err)
	}
	bm, err := c.temporalStorage.GetBlockMeta(height)
	if err != nil {
		c.log.Err(err).Int64("height", height).Msg("fail to get blockmeta")
	}
	if bm == nil {
		bm = utxo.NewBlockMeta("", height, "")
	}
	defer func() {
		// trunk-ignore(golangci-lint/govet): shadow
		if err := c.temporalStorage.SaveBlockMeta(height, bm); err != nil {
			c.log.Err(err).Msg("fail to save block metadata")
		}
	}()

	redeemTx := btcwire.NewMsgTx(btcwire.TxVersion)

	var txid string
	switch c.cfg.ChainID {
	case common.ZECChain:
		if len(payload) == 0 {
			return "", fmt.Errorf("payload is empty")
		}

		args := []any{hex.EncodeToString(payload)}

		err = c.rpc.Call(&txid, "sendrawtransaction", args...)
		if err != nil {
			c.log.Err(err).Msg("fail to call SendRawTransaction")
		}
		if txid == "" {
			c.log.Error().Msg("fail to call SendRawTransaction")
		}
	default:
		buf := bytes.NewBuffer(payload)
		if err = redeemTx.Deserialize(buf); err != nil {
			return "", fmt.Errorf("fail to deserialize payload: %w", err)
		}

		var maxFee any
		switch c.cfg.ChainID {
		case common.BTCChain:
			maxFee = 10_000_000
		}

		txid, err = c.rpc.SendRawTransaction(redeemTx, maxFee)
	}

	if txid != "" {
		bm.AddSelfTransaction(txid)
	}

	if err != nil {
		switch c.cfg.ChainID {
		case common.ZECChain:
			hash1 := sha256.Sum256(payload)
			hash2 := sha256.Sum256(hash1[:])
			final := hash2[:]

			slices.Reverse(final)

			txid = hex.EncodeToString(final)
		default:
			txid = redeemTx.TxHash().String()
		}

		if strings.Contains(err.Error(), "already in block chain") {
			c.log.Info().Str("hash", txid).Msg("broadcasted by another node")
			cacheErr := c.signerCacheManager.SetSigned(txOut.CacheHash(), txOut.CacheVault(c.GetChain()), txid)
			if cacheErr != nil {
				c.log.Err(cacheErr).Msgf("fail to mark tx out item (%+v) as signed", txOut)
			}
			return txid, nil
		}

		return "", fmt.Errorf("fail to broadcast transaction to chain: %w", err)
	}

	err = c.signerCacheManager.SetSigned(txOut.CacheHash(), txOut.CacheVault(c.GetChain()), txid)
	if err != nil {
		c.log.Err(err).Msgf("fail to mark tx out item (%+v) as signed", txOut)
	}

	return txid, nil
}
