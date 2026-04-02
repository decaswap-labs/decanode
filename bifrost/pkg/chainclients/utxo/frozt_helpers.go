package utxo

type ShieldedSigner interface {
	BuildShieldedTx(inputs []ShieldedInput, outputs []ShieldedOutput) ([]byte, error)
	ScanForDeposits(startHeight, endHeight uint64) ([]ShieldedDeposit, error)
}

type ShieldedInput struct {
	Note    []byte
	Witness []byte
	Amount  uint64
}

type ShieldedOutput struct {
	Address string
	Amount  uint64
}

type ShieldedDeposit struct {
	TxID   string
	Amount uint64
	Height uint64
}
