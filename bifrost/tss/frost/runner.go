package frost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/decaswap-labs/decanode/bifrost/tss"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

type Runner struct {
	transport Transport
	store     KeyShareStore
	partyID   uint16
	allParties []uint16
}

func NewRunner(transport Transport, store KeyShareStore, partyID uint16, allParties []uint16) *Runner {
	sorted := make([]uint16, len(allParties))
	copy(sorted, allParties)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return &Runner{
		transport:  transport,
		store:      store,
		partyID:    partyID,
		allParties: sorted,
	}
}

func (r *Runner) SignFrost(poolPubKey string, messages []string, blockHeight int64) ([]tss.FrostSignatureResult, error) {
	keyShare, err := r.store.Load(poolPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load keyshare: %w", err)
	}

	results := make([]tss.FrostSignatureResult, len(messages))

	for i, msg := range messages {
		sig, signErr := r.signSingle(poolPubKey, msg, blockHeight, keyShare)
		results[i] = tss.FrostSignatureResult{
			Msg:       msg,
			Signature: sig,
			Err:       signErr,
		}
	}

	return results, nil
}

func (r *Runner) signSingle(poolPubKey string, msg string, blockHeight int64, keyShare []byte) ([]byte, error) {
	sessionID := r.buildSessionID("sign", poolPubKey, msg, blockHeight)
	expected := len(r.allParties) - 1

	nonces, commitments, err := stubSignCommit(keyShare)
	if err != nil {
		return nil, fmt.Errorf("sign commit failed: %w", err)
	}

	err = r.transport.Broadcast(sessionID, 1, commitments)
	if err != nil {
		return nil, fmt.Errorf("broadcast commitments failed: %w", err)
	}

	allCommitments, err := r.transport.Collect(sessionID, 1, expected)
	if err != nil {
		return nil, fmt.Errorf("collect commitments failed: %w", err)
	}
	allCommitments[r.partyID] = commitments

	msgBytes, err := hex.DecodeString(msg)
	if err != nil {
		msgBytes = []byte(msg)
	}

	signingPackage, err := stubSignCreatePackage(msgBytes, allCommitments)
	if err != nil {
		return nil, fmt.Errorf("create signing package failed: %w", err)
	}

	share, err := stubSign(signingPackage, nonces, keyShare)
	if err != nil {
		return nil, fmt.Errorf("sign failed: %w", err)
	}

	err = r.transport.Broadcast(sessionID, 2, share)
	if err != nil {
		return nil, fmt.Errorf("broadcast shares failed: %w", err)
	}

	allShares, err := r.transport.Collect(sessionID, 2, expected)
	if err != nil {
		return nil, fmt.Errorf("collect shares failed: %w", err)
	}
	allShares[r.partyID] = share

	signature, err := stubSignAggregate(signingPackage, allShares)
	if err != nil {
		return nil, fmt.Errorf("aggregate signature failed: %w", err)
	}

	return signature, nil
}

func (r *Runner) KeygenFrost(keys []string, blockHeight int64) (string, types.Blame, error) {
	sessionID := r.buildSessionID("keygen", strings.Join(keys, ","), "", blockHeight)
	expected := len(keys) - 1

	secretPkg1, publicPkg1, err := stubDkgPart1(r.partyID)
	if err != nil {
		return "", types.Blame{FailReason: "dkg part1 failed"}, fmt.Errorf("dkg part1 failed: %w", err)
	}

	err = r.transport.Broadcast(sessionID, 1, publicPkg1)
	if err != nil {
		return "", types.Blame{FailReason: "broadcast round1 failed"}, fmt.Errorf("broadcast round1 failed: %w", err)
	}

	allPart1, err := r.transport.Collect(sessionID, 1, expected)
	if err != nil {
		return "", types.Blame{FailReason: "collect round1 failed"}, fmt.Errorf("collect round1 failed: %w", err)
	}
	allPart1[r.partyID] = publicPkg1

	secretPkg2, part2Packages, err := stubDkgPart2(secretPkg1, allPart1)
	if err != nil {
		return "", types.Blame{FailReason: "dkg part2 failed"}, fmt.Errorf("dkg part2 failed: %w", err)
	}

	for targetParty, pkg := range part2Packages {
		roundKey := fmt.Sprintf("%s-p2p-%d", sessionID, targetParty)
		broadcastErr := r.transport.Broadcast(roundKey, 2, pkg)
		if broadcastErr != nil {
			return "", types.Blame{FailReason: "broadcast round2 failed"}, fmt.Errorf("broadcast round2 to party %d failed: %w", targetParty, broadcastErr)
		}
	}

	myRoundKey := fmt.Sprintf("%s-p2p-%d", sessionID, r.partyID)
	allPart2, err := r.transport.Collect(myRoundKey, 2, expected)
	if err != nil {
		return "", types.Blame{FailReason: "collect round2 failed"}, fmt.Errorf("collect round2 failed: %w", err)
	}

	keyShareBytes, pubKeyBytes, err := stubDkgPart3(secretPkg2, allPart1, allPart2)
	if err != nil {
		return "", types.Blame{FailReason: "dkg part3 failed"}, fmt.Errorf("dkg part3 failed: %w", err)
	}

	pubKeyHex := hex.EncodeToString(pubKeyBytes)

	saveErr := r.store.Save(pubKeyHex, keyShareBytes)
	if saveErr != nil {
		return "", types.Blame{FailReason: "save keyshare failed"}, fmt.Errorf("save keyshare failed: %w", saveErr)
	}

	return pubKeyHex, types.Blame{}, nil
}

func (r *Runner) buildSessionID(prefix string, key string, msg string, blockHeight int64) string {
	raw := fmt.Sprintf("%s-%s-%s-%d", prefix, key, msg, blockHeight)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:16])
}
