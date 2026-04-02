package tokenlist

import (
	"encoding/json"

	"github.com/decaswap-labs/decanode/common/tokenlist/poltokens"
)

var polTokenList EVMTokenList

func init() {
	if err := json.Unmarshal(poltokens.POLTokenListRaw, &polTokenList); err != nil {
		panic(err)
	}
}

func GetPOLTokenList() EVMTokenList {
	return polTokenList
}
