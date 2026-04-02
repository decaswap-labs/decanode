# Integration Tests

Integration tests for the decanode protocol. These tests exercise the core protocol logic
using in-memory Cosmos state with mock chain clients (no real Bitcoin/ZEC nodes required).

## Running

```bash
# Run all integration tests
go test -tags mocknet -run TestIntegration ./test/integration/...

# Run a specific test suite
go test -tags mocknet -run TestIntegrationDeposit ./test/integration/...
go test -tags mocknet -run TestIntegrationSwap ./test/integration/...
go test -tags mocknet -run TestIntegrationBond ./test/integration/...
go test -tags mocknet -run TestIntegrationKeygen ./test/integration/...
```

## Structure

- `setup_test.go` - Test harness: creates in-memory app with 3 validators and mock chain clients
- `deposit_test.go` - Deposit flow: address generation, inbound observation, balance crediting
- `swap_test.go` - Swap flow: request submission, streaming chunks, rapid matching
- `bond_test.go` - Node economics: bonding, permanent lockup, fee distribution
- `keygen_test.go` - Weekly keygen cycle: FROST DKG stubs, address index reset
