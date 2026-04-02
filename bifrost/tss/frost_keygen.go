package tss

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/decaswap-labs/decanode/bifrost/thorclient"
	"github.com/decaswap-labs/decanode/common"
	"github.com/decaswap-labs/decanode/common/cosmos"
	"github.com/decaswap-labs/decanode/x/thorchain/types"
)

type FrostKeygenRunner interface {
	KeygenFrost(keys []string, blockHeight int64) (pubKey string, blame types.Blame, err error)
}

type FrostKeyGen struct {
	keys   *thorclient.Keys
	logger zerolog.Logger
	client *http.Client
	runner FrostKeygenRunner
	bridge thorclient.ThorchainBridge
}

func NewFrostKeyGen(keys *thorclient.Keys, runner FrostKeygenRunner, bridge thorclient.ThorchainBridge) (*FrostKeyGen, error) {
	if keys == nil {
		return nil, fmt.Errorf("keys is nil")
	}
	return &FrostKeyGen{
		keys:   keys,
		logger: log.With().Str("module", "frost_keygen").Logger(),
		client: &http.Client{
			Timeout: time.Second * 130,
		},
		runner: runner,
		bridge: bridge,
	}, nil
}

func (kg *FrostKeyGen) GenerateNewKey(keygenBlockHeight int64, pKeys common.PubKeys) (common.PubKeySet, []types.Blame, error) {
	if len(pKeys) == 0 {
		return common.EmptyPubKeySet, nil, nil
	}

	var blame []types.Blame

	defer func() {
		if len(blame) == 0 {
			kg.logger.Info().Int64("height", keygenBlockHeight).Msg("frost keygen success")
		} else {
			for _, b := range blame {
				blames := make([]string, 0, len(b.BlameNodes))
				for _, n := range b.BlameNodes {
					pk, err := common.NewPubKey(n.Pubkey)
					if err != nil {
						continue
					}
					acc, err := pk.GetThorAddress()
					if err != nil {
						continue
					}
					blames = append(blames, acc.String())
				}
				sort.Strings(blames)
				kg.logger.Info().
					Int64("height", keygenBlockHeight).
					Str("round", b.Round).
					Str("blames", strings.Join(blames, ", ")).
					Str("reason", b.FailReason).
					Msg("frost keygen blame")
			}
		}
	}()

	var keys []string
	for _, item := range pKeys {
		keys = append(keys, item.String())
	}

	ch := make(chan bool, 1)
	defer close(ch)
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()

	var pubKey string
	var keygenBlame types.Blame
	var err error
	go func() {
		pubKey, keygenBlame, err = kg.runner.KeygenFrost(keys, keygenBlockHeight)
		ch <- true
	}()

	select {
	case <-ch:
	case <-timer.C:
		panic("frost keygen timeout")
	}

	if err != nil {
		blame = append(blame, keygenBlame)
		return common.EmptyPubKeySet, blame, fmt.Errorf("frost keygen failed: %w", err)
	}

	if !keygenBlame.IsEmpty() {
		blame = append(blame, keygenBlame)
		return common.EmptyPubKeySet, blame, fmt.Errorf("frost keygen blame")
	}

	ecdsaPubKey, err := common.NewPubKey(pubKey)
	if err != nil {
		return common.EmptyPubKeySet, blame, fmt.Errorf("fail to create PubKey: %w", err)
	}

	_ = cosmos.AccAddress{}

	return common.NewPubKeySet(ecdsaPubKey, ecdsaPubKey), blame, nil
}
