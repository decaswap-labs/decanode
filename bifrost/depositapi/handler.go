package depositapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decaswap-labs/decanode/common"
)

type AddressDeriver interface {
	DeriveAddress(chain common.Chain, pubKey common.PubKey, index uint32) (string, error)
}

type DepositResponse struct {
	Address string `json:"address"`
	Chain   string `json:"chain"`
	Index   uint32 `json:"index"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type chainCounter struct {
	index atomic.Uint32
}

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	existing := rl.requests[key]
	filtered := make([]time.Time, 0, len(existing))
	for _, t := range existing {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) >= rl.limit {
		rl.requests[key] = filtered
		return false
	}

	rl.requests[key] = append(filtered, now)
	return true
}

var supportedChains = map[common.Chain]bool{
	common.BTCChain: true,
	common.ZECChain: true,
}

type DepositHandler struct {
	deriver  AddressDeriver
	vaultKey common.PubKey
	counters map[common.Chain]*chainCounter
	limiter  *rateLimiter
	mu       sync.RWMutex
}

func NewDepositHandler(deriver AddressDeriver, vaultKey common.PubKey) *DepositHandler {
	counters := make(map[common.Chain]*chainCounter)
	for chain := range supportedChains {
		counters[chain] = &chainCounter{}
	}

	return &DepositHandler{
		deriver:  deriver,
		vaultKey: vaultKey,
		counters: counters,
		limiter:  newRateLimiter(30, time.Minute),
	}
}

func (h *DepositHandler) SetVaultKey(pubKey common.PubKey) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vaultKey = pubKey
}

func (h *DepositHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientIP := r.RemoteAddr
	if !h.limiter.allow(clientIP) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	chainStr := r.URL.Query().Get("chain")
	if chainStr == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter: chain")
		return
	}

	chain, err := common.NewChain(chainStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid chain: %s", chainStr))
		return
	}

	if !supportedChains[chain] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported chain: %s", chainStr))
		return
	}

	counter, ok := h.counters[chain]
	if !ok {
		writeError(w, http.StatusInternalServerError, "chain counter not initialized")
		return
	}

	index := counter.index.Add(1) - 1

	h.mu.RLock()
	vaultKey := h.vaultKey
	h.mu.RUnlock()

	if vaultKey.IsEmpty() {
		writeError(w, http.StatusServiceUnavailable, "vault key not available")
		return
	}

	addr, err := h.deriver.DeriveAddress(chain, vaultKey, index)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to derive address: %v", err))
		return
	}

	resp := DepositResponse{
		Address: addr,
		Chain:   chain.String(),
		Index:   index,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}
