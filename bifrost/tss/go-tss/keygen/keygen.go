package keygen

import (
	bcrypto "github.com/binance-chain/tss-lib/crypto"

	"github.com/decaswap-labs/decanode/bifrost/p2p"
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/common"
)

type TssKeyGen interface {
	GenerateNewKey(keygenReq Request) (*bcrypto.ECPoint, error)
	GetTssKeyGenChannels() chan *p2p.Message
	GetTssCommonStruct() *common.TssCommon
}
