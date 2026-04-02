package types

import (
	. "gopkg.in/check.v1"

	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
)

type MsgAddLiquiditySuite struct{}

var _ = Suite(&MsgAddLiquiditySuite{})

func (MsgAddLiquiditySuite) TestMsgAddLiquidity(c *C) {
	addr := GetRandomBech32Addr()
	c.Check(addr.Empty(), Equals, false)
	decaAddress := GetRandomRUNEAddress()
	assetAddress := GetRandomETHAddress()
	txID := GetRandomTxHash()
	c.Check(txID.IsEmpty(), Equals, false)
	tx := common.NewTx(
		txID,
		decaAddress,
		GetRandomRUNEAddress(),
		common.Coins{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(100000000)),
		},
		common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One))},
		"",
	)
	m := NewMsgAddLiquidity(tx, common.ETHAsset, cosmos.NewUint(100000000), cosmos.NewUint(100000000), decaAddress, assetAddress, common.NoAddress, cosmos.ZeroUint(), addr)
	EnsureMsgBasicCorrect(m, c)

	inputs := []struct {
		asset     common.Asset
		r         cosmos.Uint
		amt       cosmos.Uint
		decaAddr  common.Address
		assetAddr common.Address
		txHash    common.TxID
		signer    cosmos.AccAddress
	}{
		{
			asset:     common.Asset{},
			r:         cosmos.NewUint(100000000),
			amt:       cosmos.NewUint(100000000),
			decaAddr:  decaAddress,
			assetAddr: assetAddress,
			txHash:    txID,
			signer:    addr,
		},
		{
			asset:     common.ETHAsset,
			r:         cosmos.NewUint(100000000),
			amt:       cosmos.NewUint(100000000),
			decaAddr:  common.NoAddress,
			assetAddr: common.NoAddress,
			txHash:    txID,
			signer:    addr,
		},
		{
			asset:     common.ETHAsset,
			r:         cosmos.NewUint(100000000),
			amt:       cosmos.NewUint(100000000),
			decaAddr:  decaAddress,
			assetAddr: assetAddress,
			txHash:    common.TxID(""),
			signer:    addr,
		},
		{
			asset:     common.ETHAsset,
			r:         cosmos.NewUint(100000000),
			amt:       cosmos.NewUint(100000000),
			decaAddr:  decaAddress,
			assetAddr: assetAddress,
			txHash:    txID,
			signer:    cosmos.AccAddress{},
		},
	}
	for i, item := range inputs {
		tx = common.NewTx(
			item.txHash,
			GetRandomRUNEAddress(),
			GetRandomETHAddress(),
			common.Coins{
				common.NewCoin(item.asset, item.r),
			},
			common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One))},
			"",
		)
		m = NewMsgAddLiquidity(tx, item.asset, item.r, item.amt, item.decaAddr, item.assetAddr, common.NoAddress, cosmos.ZeroUint(), item.signer)
		c.Assert(m.ValidateBasic(), NotNil, Commentf("%d) %s\n", i, m))
	}
	// If affiliate fee basis point is more than 1000 , the message should be rejected
	m1 := NewMsgAddLiquidity(tx, common.ETHAsset, cosmos.NewUint(100*common.One), cosmos.NewUint(100*common.One), GetRandomTHORAddress(), GetRandomETHAddress(), GetRandomTHORAddress(), cosmos.NewUint(1024), GetRandomBech32Addr())
	c.Assert(m1.ValidateBasic(), NotNil)

	// check that we can have zero asset and zero rune amounts
	m1 = NewMsgAddLiquidity(tx, common.ETHAsset, cosmos.ZeroUint(), cosmos.ZeroUint(), GetRandomTHORAddress(), GetRandomETHAddress(), GetRandomTHORAddress(), cosmos.ZeroUint(), GetRandomBech32Addr())
	c.Assert(m1.ValidateBasic(), IsNil)
}

func (MsgAddLiquiditySuite) TestMsgAddLiquidity_SecuredAssets(c *C) {
	testCases := []struct {
		Asset        common.Asset
		AssetAmount  cosmos.Uint
		AssetAddress common.Address
		RuneAmount   cosmos.Uint
		DecaAddress  common.Address
		TxHash       common.TxID
		Signer       cosmos.AccAddress
		Error        string
	}{
		{
			// zero asset amount
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.ZeroUint(),
			AssetAddress: GetRandomTHORAddress(),
			RuneAmount:   cosmos.ZeroUint(),
			DecaAddress:  GetRandomTHORAddress(),
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "",
		},
		{
			// zero rune amount
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.NewUint(common.One),
			AssetAddress: GetRandomTHORAddress(),
			RuneAmount:   cosmos.ZeroUint(),
			DecaAddress:  GetRandomTHORAddress(),
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "",
		},
		{
			// non zero asset and rune amount
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.NewUint(common.One),
			AssetAddress: GetRandomTHORAddress(),
			RuneAmount:   cosmos.NewUint(common.One),
			DecaAddress:  GetRandomTHORAddress(),
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "",
		},
		{
			// no asset address
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.NewUint(common.One),
			AssetAddress: common.NoAddress,
			RuneAmount:   cosmos.ZeroUint(),
			DecaAddress:  GetRandomTHORAddress(),
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "asset address cannot be empty.*",
		},
		{
			// no rune address
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.NewUint(common.One),
			AssetAddress: GetRandomTHORAddress(),
			RuneAmount:   cosmos.ZeroUint(),
			DecaAddress:  common.NoAddress,
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "rune address cannot be empty.*",
		},
		{
			// wrong asset address
			Asset:        common.BTCAsset.GetSecuredAsset(),
			AssetAmount:  cosmos.NewUint(common.One),
			AssetAddress: GetRandomBTCAddress(),
			RuneAmount:   cosmos.ZeroUint(),
			DecaAddress:  GetRandomTHORAddress(),
			TxHash:       GetRandomTxHash(),
			Signer:       GetRandomBech32Addr(),
			Error:        "asset address must be thor address.*",
		},
	}

	for _, tc := range testCases {
		msg := NewMsgAddLiquidity(
			GetRandomTx(),
			tc.Asset,
			tc.RuneAmount,
			tc.AssetAmount,
			tc.DecaAddress,
			tc.AssetAddress,
			common.NoAddress,
			cosmos.ZeroUint(),
			tc.Signer,
		)
		err := msg.ValidateBasic()
		if tc.Error == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, tc.Error)
		}
	}
}
