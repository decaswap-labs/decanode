package thorchain

import (
	"fmt"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/constants"
)

// SwitchHandler is handler to process MsgSwitch
type SwitchHandler struct {
	mgr Manager
}

// NewSwitchHandler create a new instance of SwitchHandler
func NewSwitchHandler(mgr Manager) SwitchHandler {
	return SwitchHandler{
		mgr: mgr,
	}
}

// Run is the main entry point for SwitchHandler
func (h SwitchHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgSwitch)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgSwitch failed validation", "error", err)
		return nil, err
	}
	err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgSwitch", "error", err)
		return nil, err
	}
	return &cosmos.Result{}, err
}

func (h SwitchHandler) validate(ctx cosmos.Context, msg MsgSwitch) error {
	if err := h.checkEnabled(ctx, msg.Asset); err != nil {
		return err
	}

	return msg.ValidateBasic()
}

func (h SwitchHandler) handle(_ cosmos.Context, _ MsgSwitch) error {
	return fmt.Errorf("switch feature has been removed")
}

func (h SwitchHandler) checkEnabled(ctx cosmos.Context, asset common.Asset) error {
	m, err := h.mgr.Keeper().GetMimirWithRef(ctx, constants.MimirTemplateSwitch, asset.Chain.String(), asset.Symbol.String())
	if err != nil {
		return err
	}
	if m <= 0 || m >= ctx.BlockHeight() {
		return fmt.Errorf("%s switching is not enabled", asset.String())
	}
	return nil
}
