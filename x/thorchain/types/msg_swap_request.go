package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSwapRequest{}
	_ sdk.HasValidateBasic = &MsgSwapRequest{}
)

type MsgSwapRequest struct {
	SourceAsset       common.Asset      `json:"source_asset"`
	TargetAsset       common.Asset      `json:"target_asset"`
	Amount            cosmos.Uint       `json:"amount"`
	Destination       common.Address    `json:"destination"`
	StreamingQuantity uint64            `json:"streaming_quantity,omitempty"`
	StreamingInterval uint64            `json:"streaming_interval,omitempty"`
	Signer            cosmos.AccAddress `json:"signer"`
}

func NewMsgSwapRequest(
	sourceAsset common.Asset,
	targetAsset common.Asset,
	amount cosmos.Uint,
	destination common.Address,
	streamingQuantity uint64,
	streamingInterval uint64,
	signer cosmos.AccAddress,
) *MsgSwapRequest {
	return &MsgSwapRequest{
		SourceAsset:       sourceAsset,
		TargetAsset:       targetAsset,
		Amount:            amount,
		Destination:       destination,
		StreamingQuantity: streamingQuantity,
		StreamingInterval: streamingInterval,
		Signer:            signer,
	}
}

func (m *MsgSwapRequest) ValidateBasic() error {
	if m.SourceAsset.IsEmpty() {
		return cosmos.ErrUnknownRequest("source asset cannot be empty")
	}
	if m.TargetAsset.IsEmpty() {
		return cosmos.ErrUnknownRequest("target asset cannot be empty")
	}
	if m.Amount.IsZero() {
		return cosmos.ErrUnknownRequest("amount cannot be zero")
	}
	if m.Destination.IsEmpty() {
		return cosmos.ErrUnknownRequest("destination cannot be empty")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

func (m *MsgSwapRequest) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func (m *MsgSwapRequest) ProtoMessage()                      {}
func (m *MsgSwapRequest) Reset()                             {}
func (m *MsgSwapRequest) String() string                     { return "" }
func (m *MsgSwapRequest) XXX_MessageName() string            { return "types.MsgSwapRequest" }
func (m *MsgSwapRequest) Marshal() ([]byte, error)           { return nil, nil }
func (m *MsgSwapRequest) Unmarshal([]byte) error             { return nil }
func (m *MsgSwapRequest) MarshalTo(data []byte) (int, error) { return 0, nil }
func (m *MsgSwapRequest) Size() int                          { return 0 }
