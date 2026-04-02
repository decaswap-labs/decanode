package tokenlist

import (
	"encoding/json"

	"github.com/decaswap-labs/decanode/common/tokenlist/trontokens"
)

var tronTokenList EVMTokenList

func init() {
	err := json.Unmarshal(trontokens.TRONTokenListRaw, &tronTokenList)
	if err != nil {
		panic(err)
	}
}

func GetTRONTokenList() EVMTokenList {
	return tronTokenList
}
