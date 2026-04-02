# Cross-Chain Liquidity Protocol

## Overview

A cross-chain DEX operated by a bonded 30-node co-operative. Nodes run a BFT state machine with FROST TSS signing across connected chains.

The bond asset "B" is the routing asset in all pools and can only be bought and bonded, never sold.

## Connected Chains

Any chain where FROST can control a vault:

| Chain | Method | Notes |
|-------|--------|-------|
| BTC (frobt) | Taproot / BIP32 derivation | Native Schnorr, FROST-friendly |
| ZEC (frozt) | ZIP32 + diversifiers | Native shielded transactions |
| XMR (fromt) | Custom Keccak 2-round threshold CKD | Pre-derive address batches |

ZEC and XMR provide native on-chain privacy. The protocol does not implement its own shielded pool — privacy comes from the chains themselves. See [TODO-PRIVACY.md](TODO-PRIVACY.md) for an archived protocol-level shielded pool design.

## Architecture

### Pools

Constant-product AMM pools with B on one side of every pair:

```
Pool 1: BTC : B
Pool 2: ZEC : B
Pool 3: XMR : B
```

Cross-chain swaps route through B: `BTC → B → ZEC`.

B is only tradeable within these pools. External assets trade on external markets. Arbitrageurs keep pool prices aligned with external markets through real capital flow.

### Impact Propagation

Buy pressure on B in any single pool propagates across all pools via arbitrage. More pools = more resilience. With N pools, a buy shock is distributed across N pathways, reducing per-pool depth loss.

## Protocol State

Validators maintain all state internally — no external chain needed:

```
1. Observed Deposits
   - Tracked per-chain UTXOs / transactions to vault addresses
   - Confirmed via BFT consensus

2. User Balances
   - Internal ledger of deposited assets per user

3. Liquidity Pools
   - Pool depths for each asset pair (BTC:B, ZEC:B, XMR:B)
   - Streaming swap queue
   - Rapid swap matching engine

4. Fee Accumulator
   - All fees collected in B
   - 10% dev fund allocation tracked separately
```

## Swapper Lifecycle

Two deposit modes: **swap quotes** (deposit + swap as a single intent) and **balance deposits** (park assets, act later).

### Swap Quote (Deposit + Swap in One)

Most users want to swap in a single flow. The quote API encodes the full swap intent into the deposit address — once funds arrive, the swap begins automatically:

```
1. User → GET /quote?from=BTC&to=ZEC&dest=zs1...&chunks=10&ts=...&nonce=...
2. Node verifies PoW, returns deposit address + expected output + fees
3. ← { address: "bc1q...", expected_out: "4.2 ZEC", fee: "0.003 BTC",
        expires: "2026-04-07T00:00Z", slippage_tolerance: "1%" }
4. User sends BTC to that address
5. Validators observe inbound, reach consensus
6. Streaming swap begins automatically — no second instruction needed
7. ZEC outputs FROST-signed to dest address as chunks complete
```

The swap intent (dest chain, dest address, streaming params, slippage tolerance) is bound to the deposit address at quote time. Validators know what to do when funds arrive.

- **Multiple deposits** to the same address are fine — each follows the same route and destination
- **Partial deposits** execute normally — the swap streams whatever arrived
- **Slippage tolerance** is part of the quote intent. If price moves beyond tolerance during streaming, remaining chunks pause until price recovers or user cancels

### Quote Expiry

Quotes expire at the next scheduled churn (weekly keygen). After churn, the vault key rotates and the deposit address is no longer monitored.

```
Quote response includes: { expires: "2026-04-07T00:00Z" }
- Deposits before expiry → swap executes normally
- Deposits after expiry to old vault address → still processed if 21 signers remain on old key
- Old vault addresses drain naturally as change moves to new key
```

### Balance Deposit (Hold + Act Later)

Power users and arb agents deposit without a swap intent. Funds sit in their internal balance until they submit instructions:

```
1. User → GET /deposit?chain=BTC&ts=...&nonce=...
2. Node verifies PoW, returns deposit address
3. ← { address: "bc1p..." }
4. User sends BTC to that address
5. Validators observe inbound, credit user's internal balance
6. Funds sit until user submits a swap or withdrawal
```

Balance holders can then:
- **Swap**: submit swap instruction against their balance (no new deposit needed)
- **Withdraw**: pull assets back to an external address
- **Wait**: hold balance indefinitely, act on market conditions

This enables arb agents to pre-fund balances and execute swaps with minimal latency — no deposit confirmation delay on the trade itself.

### Balance Identity and Transactions

Once a user has an on-chain balance, they can submit transactions directly to the node (swap, withdraw, cancel). Gas is paid in-kind from their balance — no need for a separate gas token.

```
User with BTC balance submits: swap BTC → ZEC, dest=zs1..., chunks=10
  → Gas fee deducted from BTC balance in-kind
  → Remaining BTC enters streaming swap
```

### Swap (From Balance)

Users with an existing balance can submit swap instructions directly:

```
User submits: swap BTC → ZEC, dest=zs1..., chunks=10, interval=~10min

Each chunk:
  1. Debit portion of user's BTC balance
  2. Execute chunk against BTC:B pool → get B
  3. Execute B against B:ZEC pool → get ZEC
  4. Queue ZEC withdrawal to destination address
```

Streaming swaps naturally spread execution over time, reducing price impact.

Rapid swap matching still applies — opposing streams are matched directly:

```
Stream A: 10 BTC → ZEC
Stream B: 5 ZEC → BTC

Matched portion executes at pool price with zero slippage.
Remainder hits the AMM.
```

### Swap Cancellation

A streaming swap can be cancelled mid-stream. Remaining unchunked balance is returned to the user's internal balance:

```
Option A: User has on-chain balance → submit cancel tx directly (gas paid in-kind)
Option B: User has no balance → send new deposit with cancel intent to quote address
```

Already-executed chunks are not reversed. Only remaining unstreamed balance is refunded.

### Withdraw

```
1. User requests withdrawal of their internal balance
2. Validators verify sufficient balance
3. Validators FROST-sign an outbound tx on the target chain
4. User receives native assets at their specified address
```

For ZEC withdrawals to z-addresses and XMR withdrawals, the transactions are natively private on-chain.

### Outbound Batching

Small withdrawals are batched by the vault to reduce on-chain fee overhead. The protocol accumulates pending outbounds and FROST-signs a single transaction with multiple outputs when efficient.

### Refunds

If a swap cannot execute (pool halted, chain offline, routing error), the deposit is refunded to the user's internal balance. User can then withdraw or retry.

### Minimum Amounts

Minimum deposit/swap: 1000 sats equivalent. Below this, the deposit is ignored (dust protection).

### Vault Migration

Deposits to old vault addresses (pre-keygen rotation) are still processed as long as 21 active signers remain on the old key. Old vault UTXOs drain naturally as change outputs move to the current vault key through normal swap activity.

## Swap Mechanics

### Streaming Swaps

Large swaps are broken into chunks executed over multiple blocks to reduce price impact:

```
10 BTC swap streamed as 100 × 0.1 BTC chunks
Each chunk executes at current pool price
Arbers rebalance between chunks
Swapper gets TWAP-like execution
```

### Rapid Swaps (Opposing Stream Matching)

Simultaneous opposite-direction streaming swaps are matched directly:

```
Stream A: 10 BTC → ZEC
Stream B: 8 ZEC → BTC

Matched:   8 BTC ↔ 8 ZEC (direct swap, zero pool impact, zero slippage)
Remainder: 2 BTC → ZEC (executes against pool)
Fees charged on both sides regardless
```

The BFT state machine acts as a matching engine. The AMM pools are the market maker of last resort.

### Block Execution Order

```
Each block:
  1. Process deposit confirmations
  2. Collect pending streaming swap chunks
  3. Match opposing flows (rapid swaps)
  4. Execute unmatched remainder against pools
  5. Update pool depths
  6. Update user balances
  7. Accrue fees in B
  8. Emit withdrawal outputs when streams complete
```

## Node Economics

### Bond Model

- 30 node slots, each earning 1/30th of all swap fees
- Nodes buy B from pools and bond it — this is the only way in
- B principal is permanently locked — nodes cannot withdraw it
- Entry cost increases over time as earlier bonds drain B from pools
- Acts as a fair launch: early = cheap but risky, late = expensive but proven

### Fee Collection

All fees are collected in B. When a swap occurs, the fee (30 bps) is taken from the swap amount and converted to B within the pool:

```
Swap: 1.00 BTC in
  → Fee: 0.003 BTC worth of B retained from the pool swap
  → 0.997 BTC effective swap input
  → B fee accumulated in protocol state
```

### Fee Distribution (At Churn)

At weekly churn, accumulated B fees are distributed:

```
Total B fees accumulated during the epoch
  → 90% to validators (split 30 ways)
  → 10% to dev fund

Each validator's share of B is then streamed (swapped via streaming swap)
into the asset of the validator's choice:

  Validator 1: stream B → BTC to bc1q...
  Validator 2: stream B → ZEC to zs1...
  Validator 3: stream B → XMR to 4...
  ...
```

Validators nominate their preferred payout asset and destination address. The protocol executes a streaming swap from B to that asset, then FROST-signs the outbound transaction. This creates natural buy-then-sell pressure on B at churn — the buy already happened during fee collection, the sell happens during distribution, netting out over time.

### Dev Fund

10% of all B fees are allocated to the dev fund. At churn, the dev fund's B is streamed (swapped) to the dev team's preferred asset and sent to the dev address:

```
Dev fund B → streaming swap to preferred asset → FROST-sign to dev address
```

The dev address and payout asset are set by governance (node vote).

### Slot Secondary Market

Nodes cannot sell B, but can sell their slot OTC. Buyer pays seller directly (any asset, off-protocol), protocol churns old node out, new node in. No pool impact. Slot value is priced on fee revenue fundamentals.

```
slot value ≈ (annual fees / 30) / discount rate
```

## Key Management

### Weekly Keygen

Fresh keygen every week. No resharing, no stale share tracking. UTXO chains naturally support this — every transaction consumes old outputs and creates new ones to the current vault key. Old UTXOs drain through normal swap activity.

```
Monday:     keygen new key across 30 nodes
Week:       swaps send change to new key
Next Monday: keygen again, clean slate
```

Benefits:
- No stale share accumulation problem
- No migration cost (change outputs migrate naturally)
- Resets address index space weekly
- Clean security boundary every cycle

### Threshold

21-of-30 (BFT 2/3 + 1). Tolerates up to 9 nodes offline for maintenance while maintaining honest majority security. Nodes co-sign their own pooled capital — structural incentive alignment.

## Deposit Addresses

### Derivation

Each chain's FROST library supports unique deposit address derivation:

| Chain | Method | Interactive? |
|-------|--------|-------------|
| BTC (frobt) | BIP32 `change/index` | No — any node derives from public key |
| ZEC (frozt) | ZIP32 + diversifiers | No — single-party derivation |
| XMR (fromt) | Custom Keccak 2-round threshold CKD | Yes — requires signer coordination |

For Monero, pre-derive batches of addresses during quiet periods to avoid live CKD rounds on every quote.

### Deposit Address API

Two endpoints, both PoW-gated:

**Swap quote** — deposit address with swap intent bound:
```
GET /quote?from=BTC&to=ZEC&dest=zs1...&chunks=10&addr=bc1q...&ts=1719900000&nonce=8a3f...

1. Node verifies PoW
2. Node assigns index, derives deposit address
3. Node records swap intent (dest chain, dest addr, streaming params) against that index
4. ← { address: "bc1p...", expected_out: "4.2 ZEC", fee: "0.003 BTC" }
5. When deposit observed → streaming swap begins automatically
```

**Balance deposit** — deposit address with no swap intent:
```
GET /deposit?chain=BTC&addr=bc1q...&ts=1719900000&nonce=8a3f...

1. Node verifies PoW
2. Node assigns index, derives deposit address
3. ← { address: "bc1p..." }
4. When deposit observed → credited to user's internal balance, no swap triggered
```

### Client-Side Proof of Work

Deposit address allocation is gated by a lightweight PoW to prevent index exhaustion attacks without requiring IP-based rate limiting.

```
proof = SHA256(chain || user_address || timestamp || nonce)
requirement: proof must have N leading zero bits
```

- **Client-side**: grind nonce until hash meets difficulty target
- **Node-side**: single SHA256 hash to verify — O(1), stateless
- **Timestamp**: unix seconds, valid within a ±5 minute window to prevent nonce stockpiling
- **No challenge round-trip**: all inputs are client-supplied, any node can verify independently
- **Difficulty N**: tunable by governance. ~16 bits ≈ 65k hashes ≈ instant on modern hardware, painful to mass-spam

### Index Management

- Indices are never recycled within a vault lifetime (prevents late-deposit misattribution)
- BIP32 supports 2^31 indices — more than sufficient for one week of activity
- Weekly keygen resets the counter to 0
- PoW gate replaces HTTP-layer rate limiting — stateless, sybil-resistant, no IP tracking

## Arbitrageur Lifecycle

### Role

Arbitrageurs are the price correction mechanism. They align pool prices with external markets by trading real capital through mispriced pools. The protocol depends on them — without arb, pools drift.

### External Only

Nodes cannot arb — their capital is bonded B, locked and non-transferable. Only external actors with real capital can rebalance pools. This is by design: arb profit is the cost the protocol pays for accurate pricing.

### Setup (Pre-Fund Balance)

Arb agents use balance deposits to pre-position capital, avoiding deposit confirmation delay on time-sensitive trades:

```
1. GET /deposit?chain=BTC → deposit address (balance-only, PoW-gated)
2. Send BTC to deposit address — credited to internal balance
3. Repeat for each chain the agent wants to arb on
   (e.g., also deposit ZEC and XMR for bidirectional arb)
```

Once funded, the agent holds multi-asset balances and can execute swaps instantly from balance.

### Monitoring

Arb agents monitor pool prices via the node API and compare against external markets:

```
GET /pools → current pool depths and implied prices
  Pool BTC:B → implied BTC price in B
  Pool ZEC:B → implied ZEC price in B
  Cross-rate BTC/ZEC = (BTC:B price) / (ZEC:B price)

Compare against external BTC/ZEC rate (Binance, Kraken, etc.)
If mispricing > fee + slippage → profitable arb opportunity
```

### Execution

When a profitable opportunity is detected, the agent swaps from balance with minimal streaming (1 chunk) for speed:

```
1. Agent detects: pool BTC/ZEC cross-rate is 2% above external market
2. Submit swap from balance: ZEC → BTC, dest=bc1q..., chunks=1
3. Swap executes immediately against balance — single chunk, no streaming delay
4. BTC output FROST-signed to agent's external address
5. Agent sells BTC on external market at higher rate
6. Profit = external rate delta - protocol fee (30 bps) - external tx costs
```

For round-trip arb (staying within the protocol):

```
1. Agent detects: BTC:B pool is cheap relative to ZEC:B pool
2. Swap BTC → ZEC from balance (routes through B internally)
3. ZEC credited to internal balance (withdraw optional)
4. Wait for price reversion, swap ZEC → BTC back
5. Profit captured entirely within internal balances
```

Round-trip arb avoids external chain fees entirely — the agent can keep capital inside the protocol indefinitely.

### Economics

```
Arb profit = |price_pool - price_external| × trade_size - fees - chain_costs

Where:
  fees         = 30 bps protocol fee per swap
  chain_costs  = deposit confirmation wait (one-time) + withdrawal tx fee
  price_pool   = implied cross-rate from constant-product pools
  price_external = market rate on CEX/DEX
```

Arb is only profitable when mispricing exceeds total costs. This sets the **minimum pricing accuracy** the protocol can achieve — pools converge to within ~30-60 bps of external markets.

### Latency Profile

```
One-time setup:
  Deposit to balance: 1-6 confirmations depending on chain (BTC ~60min, ZEC ~5min)

Per-trade (from balance):
  Swap execution: next block (~seconds)
  Withdrawal to external: batched outbound, 1-2 blocks

Round-trip from balance (no withdrawal):
  Swap execution: next block
  No external chain latency at all
```

Balance-based arb is significantly faster than deposit-per-trade arb. Serious arb agents will maintain standing balances.

### Implications for the Protocol

- **Slower convergence than CEX**: limited by on-chain confirmation times for new capital entering the system
- **Acceptable**: these are chains where users already expect confirmation waits
- **Fee sensitivity**: fees must stay low enough that arb remains profitable, or pools stay mispriced
- **Pools as slow oracles**: converge to external prices through real capital flow, not oracle feeds
- **Multi-pool arb**: mispricing in one pool creates arb across all pools (since B is the routing asset), distributing correction pressure
- **Self-balancing**: more mispricing → more arb profit → more arb activity → faster convergence

## User Flows

### One-Shot Swap (BTC → ZEC)

```
1. GET /quote?from=BTC&to=ZEC&dest=zs1...&chunks=10 → get deposit address
2. Send BTC to deposit address
3. Swap begins automatically on deposit confirmation
4. Protocol streams the swap, FROST-signs ZEC outputs to z-address
Observer sees: BTC entered vault. ZEC left vault to a shielded z-address.
```

### One-Shot Swap (BTC → XMR)

```
1. GET /quote?from=BTC&to=XMR&dest=4...&chunks=10 → get deposit address
2. Send BTC to deposit address
3. Swap begins automatically on deposit confirmation
4. Protocol streams the swap, FROST-signs XMR outputs
Observer sees: BTC entered vault. XMR tx is opaque by default.
```

### Arb Agent Flow

```
1. GET /deposit?chain=BTC → get deposit address (balance-only)
2. Send BTC to deposit address — credited to internal balance
3. Monitor pool prices for mispricing opportunity
4. Submit swap from balance: BTC → ZEC, dest=zs1..., chunks=1
5. Swap executes against balance — no deposit wait on the trade
6. Withdraw ZEC output, sell on external market
7. Repeat — balance persists across multiple trades
```

## Node Lifecycle

### Genesis Bootstrap

Pools are created first via donation (see Pool Bootstrap). Once pools exist with B liquidity, prospective nodes buy B from the pools and bond it. The first 30 nodes bond and run the initial keygen to form the first vault.

### Joining (Churn-In)

```
1. Prospective node buys B from pools, submits bond tx
2. Node enters standby queue (not part of consensus, not earning fees)
3. At next weekly keygen, if any slot is available (node leaving or slot freed):
   - Standby node included in keygen ceremony
   - If keygen succeeds → node is now active, part of consensus, earning fees
   - If keygen fails → churn doesn't happen, retry next week
4. If no slots available and no nodes leaving → no churn, keygen still runs for key rotation
```

### Active Operation

- Active nodes participate in BFT consensus and FROST signing
- Earn 1/30th of all swap fees per epoch
- Must sign when called — failure to sign incurs penalty points

### Penalty Points

Nodes that fail to participate in signing are blamed and accumulate penalty points:

```
- Each missed/failed signing round → penalty points added
- Penalty points offset fee income (net earnings reduced)
- If penalty points exceed threshold → node forcibly churned out at next keygen
- Penalty points reset on successful churn-in (clean slate)
```

This creates a gradient: occasional downtime costs money but isn't fatal. Persistent unreliability gets you removed.

### Leaving (Churn-Out)

```
1. Node signals intent to leave (or is forcibly removed via penalty threshold)
2. At next weekly keygen, node excluded from new signer set
3. New keygen runs without the departing node
4. Once keygen completes → node is no longer part of consensus
5. Node retains bonded B (permanently locked) and their slot marker
6. Slot is now available for a standby node to churn in
```

### Slot Transfer (OTC Sale)

```
1. Departing node must churn out first (not part of active set)
2. Seller and buyer agree on price off-protocol (any asset, any venue)
3. Seller submits transfer_bond tx → bond ownership moves to buyer's node
4. Buyer's node enters standby queue with the transferred bond
5. Buyer churns in at next keygen
```

### Software Upgrades

Cosmos SDK upgrade mechanism — nodes signal version support, upgrade executes at a coordinated block height when supermajority signals readiness.

### Deposit Confirmation

Deposits are confirmed when 67% of active validators observe the same inbound transaction. No single node can fabricate a deposit.

### Lifecycle Summary

| Scenario | Action |
|----------|--------|
| Protocol thriving | Nodes earn fees in B, stream to preferred asset at churn |
| Protocol dying | Nodes vote to dissolve, TSS-sign all assets back to themselves |
| Node wants out | Signal leave, churned out at next keygen, can transfer_bond |
| Node misbehaving | Penalty points accumulate, offset fees, eventually forcibly churned out |
| Node dies (keys lost) | Capital locked forever (deflationary burn), slot eventually freed |
| Node maintenance | Threshold (21/30) covers temporary downtime |
| Keygen fails | Churn doesn't happen, retry next week, active set unchanged |
| No churn needed | Weekly keygen still runs for key rotation if nodes want in/out, skipped otherwise |

## Security Properties

- **Self-securing:** nodes guard their own bonded capital
- **No sell pressure:** B cannot be sold directly, only earned and distributed at churn
- **No mercenary capital:** only node operators have capital in the system
- **No inflation tax:** fees are real swap revenue, not token emissions
- **Natural quality filter:** increasing slot cost self-selects for serious operators
- **Graceful shutdown:** nodes can always vote to dissolve and recover pooled assets
- **Native chain privacy:** ZEC (shielded txs) and XMR (opaque by default) provide privacy at the chain level

## Component Stack

| Component | Language | Notes |
|---|---|---|
| FROST keygen/signing | Rust (frozt-lib, frobt, fromt) | Exists |
| BFT state machine | Go or Rust | Consensus on state transitions |
| Chain watchers | Go | One per connected chain (BTC, ZEC, XMR) |
| AMM + streaming engine | Go or Rust | Pool math, stream scheduling, rapid matching |
| Deposit API | Go | HTTP endpoint, address derivation |

## Adding New Chains

A new chain can be added at any time via node governance vote. When a new chain is added, the protocol bootstraps its pool:

### Pool Bootstrap

```
Existing pools: N (e.g. BTC:B, ZEC:B, XMR:B → N=3)
New chain: LTC

1. Protocol mints 1/N of existing B supply as new B
   (if total B across all pools = 300, mint 100 new B)

2. New pool created: LTC:B with the minted B on the B side

3. First depositor sends LTC to the new vault
   - The minted B is streamed (sold) into the new LTC deposit
   - This "buys" LTC to a target depth of total_liquidity / (N+1)
   - Streaming prevents massive price impact

4. Result: LTC:B pool is bootstrapped at fair depth
   - All pools now hold roughly equal B depth
   - Arbitrageurs correct any mispricing across pools
```

There are no LPs. The initial deposit is a **donation** — the depositor does not receive pool shares, B tokens, or any claim on the liquidity. This is the cost of bootstrapping a new chain. In practice, the dev fund or node operators fund this to grow the protocol.

### Safeguards

- Node supermajority (21/30) required to approve new chain addition
- New FROST keygen required for the new chain's vault
- Minted B is not bonded — it enters circulation in the new pool
- Streaming the mint prevents a single-block shock to B pricing
- Pool must reach minimum depth before swaps are enabled
- Donation is irreversible — no withdrawal mechanism for bootstrap liquidity

## Build Order

1. FROST vaults on BTC + ZEC + XMR (deposit → return loop)
2. BFT state machine + pool state (consensus on state transitions)
3. AMM with routing asset B (functional swaps)
4. Streaming swaps + rapid swap matching
5. Fee collection in B + churn distribution + dev fund
6. Additional chains
