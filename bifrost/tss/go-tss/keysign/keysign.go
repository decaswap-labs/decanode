package keysign

import (
	bc "github.com/binance-chain/tss-lib/common"

	"github.com/decaswap-labs/decanode/bifrost/p2p"
	"github.com/decaswap-labs/decanode/bifrost/p2p/storage"
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/common"
)

type TssKeySign interface {
	GetTssKeySignChannels() chan *p2p.Message
	GetTssCommonStruct() *common.TssCommon
	SignMessage(msgToSign [][]byte, localStateItem storage.KeygenLocalState, parties []string) ([]*bc.SignatureData, error)
}
