package tokenlist

import (
	"encoding/json"

	"github.com/decaswap-labs/decanode/common/tokenlist/basetokens"
)

var baseTokenList EVMTokenList

func init() {
	if err := json.Unmarshal(basetokens.BASETokenListRaw, &baseTokenList); err != nil {
		panic(err)
	}
}

func GetBASETokenList() EVMTokenList {
	return baseTokenList
}
