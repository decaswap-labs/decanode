# Plan: Add Monero (XMR) to ThorChain Bifrost

## Context

frosty-lib's `fromt` crate provides a complete threshold CLSAG signing protocol for Monero. The Go bindings (`frosty-lib/go/fromt/`) already expose DKG, signing, address derivation, key images, and the 3-phase spend protocol (`SpendPreprocess` / `SpendSign` / `SpendComplete`). ThorChain's Bifrost needs a new Monero chain client to integrate this.

**Key challenge**: Monero is fundamentally different from all existing Bifrost chains:
- No transparent UTXO set -- must scan blocks with private view key
- No memo field -- must use subaddresses for deposit identification
- Multi-round signing (3-phase CLSAG) -- cannot use simple `RemoteSign(msg)`
- Transaction construction requires daemon RPC (decoy selection, owned outputs)

---

## Phase 1: Missing Go Bindings in fromt

Two FFI functions have C headers but no Go wrappers yet.

### Files to modify
- `frosty-lib/go/fromt/fromt.go` -- add `ScanBalance()` and `SpendPrepare()`

### Functions to add

```
ScanBalance(keyShare, daemonURL string, birthday uint64, spendKey []byte) (balance uint64, numOutputs uint32, err error)
  wraps: fromt_scan_balance (header line 159)

SpendPrepare(keyShare []byte, daemonURL, recipient string, amount, birthday uint64, excludedOffsets, spendKey []byte) (signableTx, spentOffsets []byte, err error)
  wraps: fromt_spend_prepare (header line 167)
```

Follow the existing `cGoSlice` + `copyBuffer` pattern used throughout fromt.go.

### Tests
- `frosty-lib/go/fromt/fromt_test.go` -- add test for ScanBalance + SpendPrepare (requires regtest monerod)

---

## Phase 2: Chain Constants in thornode

### Files to modify

**`thornode/common/chain.go`**:
- Add `XMRChain = Chain("XMR")` constant
- Add to `AllChains` slice
- `GetSigningAlgo()`: return `SigningAlgoEd25519` (like SOL)
- `GetGasAsset()`: return XMR asset
- `GetGasAssetDecimal()`: return `12` (piconero)
- `ApproximateBlockMilliseconds()`: return `120_000` (2 min)
- `DustThreshold()`: return appropriate minimum
- `MaxMemoLength()`: return `0` (Monero has no memo)
- Do NOT add to `GetUTXOChains()` or `GetEVMChains()`

**`thornode/common/asset.go`**:
- Add `XMRAsset` definition

**`thornode/common/pubkey.go`**:
- Add `XMRChain` case in `GetAddress()` -- return empty/error since Monero address derivation requires the view key from the keyshare (handled by chain client's `GetAddress(poolPubKey)` instead)

---

## Phase 3: Monero Chain Client

### New directory: `thornode/bifrost/pkg/chainclients/monero/`

### Files to create

**`xmr_client.go`** -- main client implementing `ChainClient` interface
```go
type XMRClient struct {
    cfg             config.BifrostChainConfiguration
    bridge          thorclient.ThorchainBridge
    tssKeyManager   tss.ThorchainKeyManager
    keyShares       map[string][]byte  // poolPubKey -> fromt keyshare bundle
    scanner         *XMRScanner
    rpcClient       *MoneroRPC
    daemonURL       string
    stopchan        chan struct{}
}
```

Key methods:
- `Start()` -- start scanner, load keyshares
- `Stop()`
- `GetHeight()` -- call `get_block_count` RPC
- `GetAddress(poolPubKey)` -- call `fromt.DeriveAddress(keyShare)`
- `GetAccount(poolPubKey)` -- call `fromt.ScanBalance()` to get vault balance
- `SignTx()` -- the critical method, see Phase 4
- `BroadcastTx()` -- call `sendrawtransaction` daemon RPC

**`rpc.go`** -- Monero daemon JSON-RPC client
- `get_block_count`, `get_block`, `get_transactions`, `sendrawtransaction`, `is_key_image_spent`
- Standard HTTP JSON-RPC 2.0 wrapper

**`xmr_scanner.go`** -- output scanner using view key
- On startup: full scan from birthday via `fromt.ScanBalance()`
- Ongoing: incremental per-block scanning
- Deposit identification via subaddress matching (see below)
- Store owned outputs + last scanned height in LevelDB

**`xmr_signer.go`** -- signing coordinator (see Phase 4)

---

## Phase 4: Signing Coordinator (Critical Path)

The 3-phase CLSAG spend protocol must be coordinated across Bifrost nodes:

```
1. SpendPrepare(keyShare, daemon, recipient, amount, ...) -> signableTx
2. SpendPreprocess(keyShare, signableTx) -> handle + preprocess bytes
3. SpendSign(handle, preprocessesMap) -> handle + share bytes
4. SpendComplete(handle, sharesMap) -> raw signed tx bytes
```

### Coordination approach

Reuse go-tss's P2P messaging layer but with a custom protocol:

1. **Leader election**: The node that receives the `TxOutItem` from THORChain acts as coordinator
2. **Phase 1 (local)**: Coordinator calls `SpendPrepare()` to build the signable tx
3. **Phase 2 (broadcast)**: Coordinator sends `signableTx` to all signing parties. Each party calls `SpendPreprocess()` and returns their preprocess bytes
4. **Phase 3 (broadcast)**: Each party receives all preprocesses, calls `SpendSign()`, returns share bytes
5. **Phase 4 (aggregate)**: Each party receives all shares, calls `SpendComplete()` to get the raw signed tx

This maps to a 3-round P2P exchange, similar to DKG but for signing. The go-tss `KeySign` request/response mechanism can be extended or a parallel coordinator built using the same libp2p transport.

### Alternative: Session-based API
The C header already declares `fromt_sign_session_*` functions (lines 219-245) but these are for standard Ed25519 FROST signing, NOT the CLSAG spend protocol. A spend session API would need to be added to fromt first. For initial implementation, use the direct 3-phase API with a custom coordinator.

---

## Phase 5: Deposit Identification (Subaddress Scheme)

Since Monero has no memo field, use subaddresses to identify deposits:

- Each vault derives subaddresses via `fromt.DeriveSubaddress(keyShare, account, index)`
- **Account 0, Index 0**: Main vault address (default)
- **Account 1+**: Reserved for specific pool/action types
- The `/inbound_address` endpoint returns the appropriate subaddress
- Scanner matches incoming outputs against known subaddresses to identify the action

### Subaddress registry
- Store mapping of `(account, index) -> pool action` in LevelDB
- Generate new subaddresses on demand when THORNode requests inbound addresses

---

## Phase 6: Configuration & Registration

**`thornode/config/default.yaml`**:
```yaml
xmr:
  disabled: true
  chain_id: XMR
  rpc_host: http://localhost:18081
  block_scanner:
    chain_id: XMR
    observation_flexibility_blocks: 10
  solvency_blocks: 30
```

**`thornode/bifrost/pkg/chainclients/loadchains.go`**:
- Add `case common.XMRChain: return monero.NewXMRClient(...)`

---

## Phase 7: Testing Strategy

### Unit tests
- `fromt` Go bindings: DKG -> SpendPrepare -> SpendPreprocess -> SpendSign -> SpendComplete round-trip
- Chain constants: `XMRChain.GetSigningAlgo()`, asset definitions
- RPC client: mock JSON-RPC responses
- Scanner: mock block data with known outputs

### Integration tests (require regtest monerod)
- Full flow: keygen -> derive address -> fund -> scan -> sign outbound -> broadcast
- Multi-party spend with 2-of-3 threshold
- Subaddress generation and deposit matching

### Mocknet
- Add `monerod --regtest` container to docker-compose
- Configure Bifrost XMR client to connect to regtest
- Test swap flows: RUNE <-> XMR

---

## Dependency Order

```
Phase 1 (Go bindings)  ─┐
Phase 2 (chain constants) ─┤─> Phase 3 (chain client) ─> Phase 4 (signing) ─> Phase 6 (config)
                           │                            ─> Phase 5 (subaddresses)
                           └─> Phase 7 (tests, ongoing)
```

**Critical path**: Phase 1 -> Phase 3 -> Phase 4 (signing coordinator)

The signing coordinator (Phase 4) is the hardest piece -- it requires multi-round P2P coordination that doesn't fit the existing `RemoteSign()` pattern.

---

## What Exists vs What's New

### Already exists (reuse)
- `fromt` Go bindings: DKG, SpendPreprocess/Sign/Complete, DeriveAddress, DeriveSubaddress, KeyImage, KeyShare inspection
- `ChainClient` interface and Bifrost signer/observer pipeline
- Block scanner storage (LevelDB), signer cache, solvency reporting
- go-tss P2P transport layer
- fromt-lib.h C header for ScanBalance/SpendPrepare FFI

### Must be built
- Go wrappers for `ScanBalance` and `SpendPrepare` (small, follows existing pattern)
- `XMRClient` implementing `ChainClient` (~500-800 lines)
- Monero daemon RPC client (~200 lines)
- View-key-based output scanner (~300 lines)
- 3-phase CLSAG signing coordinator (~400 lines)
- Subaddress deposit registry (~150 lines)
- Chain constants and config (~50 lines across files)
