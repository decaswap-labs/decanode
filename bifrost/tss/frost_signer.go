package tss

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/cometbft/cometbft/crypto"
	sdkTypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tendermint/btcd/btcec"

	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

const (
	frostKeysignTimeout    = 5
	maxFrostSignPerRequest = 15
)

type FrostCeremonyRunner interface {
	SignFrost(poolPubKey string, messages []string, blockHeight int64) ([]FrostSignatureResult, error)
}

type FrostSignatureResult struct {
	Msg       string
	Signature []byte
	Err       error
}

type FrostKeySign struct {
	logger   zerolog.Logger
	runner   FrostCeremonyRunner
	bridge   thorclient.ThorchainBridge
	wg       *sync.WaitGroup
	taskQ    chan *frostSignTask
	done     chan struct{}
}

type frostSignTask struct {
	PoolPubKey string
	Msg        string
	Resp       chan frostSignResult
}

type frostSignResult struct {
	Signature []byte
	Err       error
}

func NewFrostKeySign(runner FrostCeremonyRunner, bridge thorclient.ThorchainBridge) (*FrostKeySign, error) {
	return &FrostKeySign{
		runner: runner,
		bridge: bridge,
		logger: log.With().Str("module", "frost_signer").Logger(),
		wg:     &sync.WaitGroup{},
		taskQ:  make(chan *frostSignTask),
		done:   make(chan struct{}),
	}, nil
}

func (s *FrostKeySign) GetPrivKey() crypto.PrivKey {
	return nil
}

func (s *FrostKeySign) GetAddr() sdkTypes.AccAddress {
	return nil
}

func (s *FrostKeySign) ExportAsMnemonic() (string, error) {
	return "", nil
}

func (s *FrostKeySign) ExportAsPrivateKey() (string, error) {
	return "", nil
}

func (s *FrostKeySign) ExportAsKeyStore(_ string) (*EncryptedKeyJSON, error) {
	return nil, nil
}

func (s *FrostKeySign) Start() {
	s.wg.Add(1)
	go s.processSignTasks()
}

func (s *FrostKeySign) Stop() {
	close(s.done)
	s.wg.Wait()
	close(s.taskQ)
}

func (s *FrostKeySign) RemoteSign(msg []byte, _ common.SigningAlgo, poolPubKey string) ([]byte, []byte, error) {
	if len(msg) == 0 {
		return nil, nil, nil
	}

	encodedMsg := base64.StdEncoding.EncodeToString(msg)
	task := frostSignTask{
		PoolPubKey: poolPubKey,
		Msg:        encodedMsg,
		Resp:       make(chan frostSignResult, 1),
	}
	s.taskQ <- &task

	select {
	case resp := <-task.Resp:
		if resp.Err != nil {
			return nil, nil, fmt.Errorf("frost sign failed: %w", resp.Err)
		}
		if len(resp.Signature) == 0 {
			return nil, nil, nil
		}
		sig := resp.Signature
		if len(sig) == 64 {
			R := new(big.Int).SetBytes(sig[:32])
			S := new(big.Int).SetBytes(sig[32:])
			N := btcec.S256().N
			halfOrder := new(big.Int).Rsh(N, 1)
			if S.Cmp(halfOrder) == 1 {
				S.Sub(N, S)
			}
			sigBytes := make([]byte, 64)
			rBytes := R.Bytes()
			sBytes := S.Bytes()
			copy(sigBytes[32-len(rBytes):32], rBytes)
			copy(sigBytes[64-len(sBytes):64], sBytes)
			return sigBytes, nil, nil
		}
		return sig, nil, nil
	case <-time.After(time.Minute * frostKeysignTimeout):
		return nil, nil, fmt.Errorf("TIMEOUT: frost sign after %d minutes", frostKeysignTimeout)
	}
}

func (s *FrostKeySign) processSignTasks() {
	defer s.wg.Done()
	tasks := make(map[string][]*frostSignTask)
	taskLock := sync.Mutex{}
	for {
		select {
		case <-s.done:
			return
		case t := <-s.taskQ:
			taskLock.Lock()
			tasks[t.PoolPubKey] = append(tasks[t.PoolPubKey], t)
			taskLock.Unlock()
		case <-time.After(time.Second):
			taskLock.Lock()
			for k, v := range tasks {
				if len(v) == 0 {
					delete(tasks, k)
					continue
				}
				sort.SliceStable(v, func(i, j int) bool {
					return v[i].Msg < v[j].Msg
				})
				total := len(v)
				if total > maxFrostSignPerRequest {
					total = maxFrostSignPerRequest
				}
				batch := v[:total]
				tasks[k] = v[total:]
				s.wg.Add(1)
				go s.executeFrostSign(k, batch)
			}
			taskLock.Unlock()
		}
	}
}

func (s *FrostKeySign) executeFrostSign(poolPubKey string, tasks []*frostSignTask) {
	defer s.wg.Done()

	var messages []string
	for _, t := range tasks {
		messages = append(messages, t.Msg)
	}

	blockHeight, err := s.bridge.GetBlockHeight()
	if err != nil {
		s.failTasks(tasks, fmt.Errorf("fail to get block height: %w", err))
		return
	}

	results, err := s.runner.SignFrost(poolPubKey, messages, blockHeight/20*20)
	if err != nil {
		blame := types.Blame{
			FailReason: err.Error(),
		}
		s.failTasks(tasks, NewKeysignError(blame))
		return
	}

	for _, t := range tasks {
		found := false
		for _, r := range results {
			if t.Msg == r.Msg {
				t.Resp <- frostSignResult{
					Signature: r.Signature,
					Err:       r.Err,
				}
				found = true
				break
			}
		}
		if !found {
			t.Resp <- frostSignResult{
				Err: fmt.Errorf("no signature for message %s", t.Msg),
			}
		}
	}
}

func (s *FrostKeySign) failTasks(tasks []*frostSignTask, err error) {
	for _, t := range tasks {
		select {
		case t.Resp <- frostSignResult{Err: err}:
		case <-time.After(time.Second):
			continue
		}
	}
}
