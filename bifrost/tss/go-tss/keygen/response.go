package keygen

import (
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/blame"
	"github.com/decaswap-labs/decanode/bifrost/tss/go-tss/common"
	tcommon "github.com/decaswap-labs/decanode/common"
)

// Response keygen response
type Response struct {
	Algo        tcommon.SigningAlgo `json:"algo"`
	PubKey      string              `json:"pub_key"`
	PoolAddress string              `json:"pool_address"`
	Status      common.Status       `json:"status"`
	Blame       blame.Blame         `json:"blame"`
}

// NewResponse create a new instance of keygen.Response
func NewResponse(algo tcommon.SigningAlgo, pk, addr string, status common.Status, blame blame.Blame) Response {
	return Response{
		Algo:        algo,
		PubKey:      pk,
		PoolAddress: addr,
		Status:      status,
		Blame:       blame,
	}
}
