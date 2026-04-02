package utxo

import "fmt"

type TaprootSigner interface {
	ComputeSighash(rawTx, prevouts []byte, inputIndex uint32) ([]byte, error)
	AttachWitness(rawTx []byte, inputIndex uint32, signature []byte) ([]byte, error)
}

type stubTaprootSigner struct{}

func (s *stubTaprootSigner) ComputeSighash(_, _ []byte, _ uint32) ([]byte, error) {
	return nil, fmt.Errorf("taproot signer not available: frobt not linked")
}

func (s *stubTaprootSigner) AttachWitness(_ []byte, _ uint32, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("taproot signer not available: frobt not linked")
}
