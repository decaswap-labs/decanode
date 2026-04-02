package thorchain

import (
	"errors"

	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

func (qs queryServer) queryReferenceMemo(_ cosmos.Context, _ *types.QueryReferenceMemoRequest) (*types.QueryReferenceMemoResponse, error) {
	return nil, errors.New("reference memos are not supported")
}

func (qs queryServer) queryReferenceMemoByHash(_ cosmos.Context, _ *types.QueryReferenceMemoByHashRequest) (*types.QueryReferenceMemoByHashResponse, error) {
	return nil, errors.New("reference memos are not supported")
}

func (qs queryServer) queryReferenceMemoPreflight(_ cosmos.Context, _ *types.QueryReferenceMemoPreflightRequest) (*types.QueryReferenceMemoPreflightResponse, error) {
	return nil, errors.New("reference memos are not supported")
}
