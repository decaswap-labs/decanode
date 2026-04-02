package utxo

import (
	"bytes"
	"encoding/hex"
	"fmt"

	btcjson "github.com/btcsuite/btcd/btcjson"
	btcchaincfg "github.com/btcsuite/btcd/chaincfg"
	btcwire "github.com/btcsuite/btcd/wire"
	btctxscript "github.com/decaswap-labs/decanode/bifrost/txscript/txscript"

	stypes "github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	"github.com/decaswap-labs/decanode/common"
)

func (c *Client) getChainCfgBTC() *btcchaincfg.Params {
	switch common.CurrentChainNetwork {
	case common.MockNet:
		return &btcchaincfg.RegressionNetParams
	case common.TestNet:
		return &btcchaincfg.TestNet3Params
	case common.MainNet, common.StageNet, common.ChainNet:
		return &btcchaincfg.MainNetParams
	default:
		c.log.Fatal().Msg("unsupported network")
		return nil
	}
}

func (c *Client) signUTXOBTC(redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	if !tx.VaultPubKey.Equals(c.nodePubKey) {
		return c.signUTXOBTCTaproot(redeemTx, tx, amount, sourceScript, idx)
	}

	return c.signUTXOBTCSegwit(redeemTx, tx, amount, sourceScript, idx)
}

func (c *Client) signUTXOBTCSegwit(redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	sigHashes := btctxscript.NewTxSigHashes(redeemTx)
	signable := btctxscript.NewPrivateKeySignable(c.nodePrivKey)

	witness, err := btctxscript.WitnessSignature(redeemTx, sigHashes, idx, amount, sourceScript, btctxscript.SigHashAll, signable, true)
	if err != nil {
		return fmt.Errorf("fail to get witness: %w", err)
	}

	redeemTx.TxIn[idx].Witness = witness
	flag := btctxscript.StandardVerifyFlags
	engine, err := btctxscript.NewEngine(sourceScript, redeemTx, idx, flag, nil, nil, amount)
	if err != nil {
		return fmt.Errorf("fail to create engine: %w", err)
	}
	err = engine.Execute()
	if err != nil {
		if btctxscript.IsErrorCode(err, btctxscript.ErrNullFail) {
			c.log.Error().
				Err(err).
				Int("input_idx", idx).
				Msg("NULLFAIL FAILSAFE TRIGGERED - This should not happen! Investigate immediately!")
			return nil
		}
		return fmt.Errorf("fail to execute the script: %w", err)
	}
	return nil
}

func (c *Client) signUTXOBTCTaproot(redeemTx *btcwire.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) error {
	taprootSigner := c.getTaprootSigner()

	var rawTx bytes.Buffer
	err := redeemTx.Serialize(&rawTx)
	if err != nil {
		return fmt.Errorf("fail to serialize tx for taproot sighash: %w", err)
	}

	prevouts := serializePrevouts(redeemTx, amount, sourceScript)

	sighash, err := taprootSigner.ComputeSighash(rawTx.Bytes(), prevouts, uint32(idx))
	if err != nil {
		return fmt.Errorf("fail to compute taproot sighash: %w", err)
	}

	signable := newTssSignableBTC(tx.VaultPubKey, c.tssKeySigner, c.log)
	sig, err := signable.Sign(sighash)
	if err != nil {
		return fmt.Errorf("fail to sign taproot sighash: %w", err)
	}

	signedRaw, err := taprootSigner.AttachWitness(rawTx.Bytes(), uint32(idx), sig.Serialize())
	if err != nil {
		return fmt.Errorf("fail to attach taproot witness: %w", err)
	}

	var signed btcwire.MsgTx
	err = signed.Deserialize(bytes.NewReader(signedRaw))
	if err != nil {
		return fmt.Errorf("fail to deserialize signed taproot tx: %w", err)
	}

	redeemTx.TxIn[idx].Witness = signed.TxIn[idx].Witness
	return nil
}

func (c *Client) getTaprootSigner() TaprootSigner {
	return &stubTaprootSigner{}
}

func serializePrevouts(redeemTx *btcwire.MsgTx, amount int64, sourceScript []byte) []byte {
	var buf bytes.Buffer
	for range redeemTx.TxIn {
		amtBytes := make([]byte, 8)
		amtBytes[0] = byte(amount)
		amtBytes[1] = byte(amount >> 8)
		amtBytes[2] = byte(amount >> 16)
		amtBytes[3] = byte(amount >> 24)
		amtBytes[4] = byte(amount >> 32)
		amtBytes[5] = byte(amount >> 40)
		amtBytes[6] = byte(amount >> 48)
		amtBytes[7] = byte(amount >> 56)
		buf.Write(amtBytes)
		buf.Write(sourceScript)
	}
	return buf.Bytes()
}

func (c *Client) getAddressesFromScriptPubKeyBTC(scriptPubKey btcjson.ScriptPubKeyResult) []string {
	addresses := scriptPubKey.Addresses
	if len(addresses) > 0 {
		return addresses
	}

	if len(scriptPubKey.Hex) == 0 {
		return nil
	}
	buf, err := hex.DecodeString(scriptPubKey.Hex)
	if err != nil {
		c.log.Err(err).Msg("fail to hex decode script pub key")
		return nil
	}
	_, extractedAddresses, _, err := btctxscript.ExtractPkScriptAddrs(buf, c.getChainCfgBTC())
	if err != nil {
		c.log.Err(err).Msg("fail to extract addresses from script pub key")
		return nil
	}
	for _, item := range extractedAddresses {
		addresses = append(addresses, item.String())
	}
	return addresses
}
