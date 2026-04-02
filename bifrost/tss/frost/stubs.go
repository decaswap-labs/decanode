//go:build !frosty

package frost

import "fmt"

var errStub = fmt.Errorf("frosty-lib not linked: build with -tags frosty")

func stubSignCommit(keyShare []byte) (noncesHandle interface{}, commitments []byte, err error) {
	return nil, nil, errStub
}

func stubSignCreatePackage(msg []byte, allCommitments map[uint16][]byte) ([]byte, error) {
	return nil, errStub
}

func stubSign(signingPackage []byte, nonces interface{}, keyShare []byte) ([]byte, error) {
	return nil, errStub
}

func stubSignAggregate(signingPackage []byte, allShares map[uint16][]byte) ([]byte, error) {
	return nil, errStub
}

func stubDkgPart1(partyID uint16) (secretPackage []byte, publicPackage []byte, err error) {
	return nil, nil, errStub
}

func stubDkgPart2(secretPackage []byte, allPart1Packages map[uint16][]byte) (secretPackage2 []byte, packages map[uint16][]byte, err error) {
	return nil, nil, errStub
}

func stubDkgPart3(secretPackage2 []byte, allPart1Packages map[uint16][]byte, allPart2Packages map[uint16][]byte) (keyShare []byte, pubKey []byte, err error) {
	return nil, nil, errStub
}
