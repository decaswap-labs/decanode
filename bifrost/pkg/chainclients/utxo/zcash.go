package utxo

import (
	"fmt"

	btcchaincfg "github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/decaswap-labs/decanode/bifrost/pkg/chainclients/utxo/zecutil"
	stypes "github.com/decaswap-labs/decanode/bifrost/thorclient/types"
	btctxscript "github.com/decaswap-labs/decanode/bifrost/txscript/txscript"
	"github.com/decaswap-labs/decanode/common"
)

func (c *Client) getChainCfgZEC() *btcchaincfg.Params {
	switch common.CurrentChainNetwork {
	case common.MockNet:
		return &btcchaincfg.Params{Name: "testnet3"}
	default:
		return &btcchaincfg.Params{Name: "mainnet"}
	}
}

func (c *Client) signUTXOZEC(zecTx *zecutil.MsgTx, tx stypes.TxOutItem, amount int64, sourceScript []byte, idx int) (err error) {
	var signable btctxscript.Signable
	if tx.VaultPubKey.Equals(c.nodePubKey) {
		signable = btctxscript.NewPrivateKeySignable(c.nodePrivKey)
	} else {
		signable = newTssSignableBTC(tx.VaultPubKey, c.tssKeySigner, c.log)
	}

	var cache *zecutil.TxSigHashes
	if cache, err = zecutil.NewTxSigHashes(zecTx); err != nil {
		return err
	}

	blakeHash, err := zecutil.Blake2bSignatureHash(sourceScript, cache, txscript.SigHashAll, zecTx, idx, amount)
	if err != nil {
		return err
	}

	signature, err := signable.Sign(blakeHash)
	if err != nil {
		return err
	}

	if !signature.Verify(blakeHash, signable.GetPubKey()) {
		return fmt.Errorf("signature verify failed")
	}

	pkData := signable.GetPubKey().SerializeCompressed()

	sigScript, err := btctxscript.NewScriptBuilder().
		AddData(append(signature.Serialize(), byte(txscript.SigHashAll))).
		AddData(pkData).
		Script()
	if err != nil {
		return err
	}

	zecTx.TxIn[idx].SignatureScript = sigScript

	return nil
}
