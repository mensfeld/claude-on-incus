# Testing OVN Network Isolation

## Overview

The network isolation features (restricted and allowlist modes) require OVN (Open Virtual Network) to function. This document explains how OVN functionality is tested both in CI and locally.

## What We Test

### Unit Tests (Always Run in CI)

The unit tests in `internal/network/acl_test.go` cover the critical logic:

1. **ACL Rule Generation** - Tests that rules are generated correctly
2. **Rule Ordering** - **CRITICAL**: OVN evaluates rules in order, so we verify:
   - Restricted mode: REJECT rules come before ALLOW (block specific, allow rest)
   - Allowlist mode: ALLOW rules come before REJECT (allow specific, block RFC1918)
3. **IP Deduplication** - Multiple domains can resolve to same IP (e.g., api/platform.anthropic.com)
4. **No Catch-All Reject** - The `0.0.0.0/0` reject rule breaks OVN routing
5. **RFC1918 Blocking** - Private network ranges are properly blocked

These tests caught the bugs that were preventing network connectivity:
- ❌ Wrong rule ordering → packets matched wrong rule first
- ❌ Missing ingress rules → response traffic blocked
- ❌ Catch-all reject → gateway unreachable

### Integration Tests (CI + Local)

Full OVN integration testing requires:
- OVN northbound/southbound databases
- Open vSwitch configured with OVN
- Incus with OVN network
- Containers with ACLs applied

**These now run in CI automatically!** The CI workflow sets up a complete OVN environment and tests:
- Restricted mode allows internet, blocks RFC1918
- Allowlist mode allows only specified domains
- RFC1918 networks are always blocked

## Running Tests

### Unit Tests (Fast, No OVN Required)

```bash
# Run all network tests
go test -v ./internal/network/

# Run just ACL tests
go test -v ./internal/network/ -run TestBuild

# With coverage
go test -coverprofile=coverage.out ./internal/network/
go tool cover -html=coverage.out
```

### Manual Integration Testing (Requires OVN)

```bash
# 1. Set up OVN (see README.md OVN setup section)
sudo apt install ovn-host ovn-central
# ... follow OVN setup steps ...

# 2. Test restricted mode
./coi shell --network=restricted
# Inside container:
curl -I https://registry.npmjs.org/  # Should work
ping 10.0.0.1                        # Should fail (RFC1918 blocked)

# 3. Test allowlist mode
./coi shell --network=allowlist
# Inside container:
curl -I https://registry.npmjs.org/  # Should work (in allowed_domains)
curl -I https://google.com/          # Should fail (not in allowlist)
ping 10.0.0.1                        # Should fail (RFC1918 blocked)
```

## Why We NOW Run OVN in CI

After the recent connectivity bugs, we decided the integration tests ARE worth it:

### What OVN Integration Tests Catch

1. **Actual connectivity issues** - The bugs we fixed would have been caught
2. **ACL application** - Verifies rules are applied correctly to containers
3. **DNS resolution** - Tests that systemd-resolved works with OVN
4. **Gateway routing** - Ensures containers can reach the OVN gateway
5. **End-to-end flow** - Full workflow from container start to network access

### CI Setup

The CI now:
1. ✅ Installs OVN packages (ovn-host, ovn-central)
2. ✅ Configures OVN databases and Open vSwitch
3. ✅ Creates OVN network in Incus
4. ✅ Runs full integration tests (~3 minutes total)

**Trade-off**: Worth the time cost because these tests catch real connectivity issues that unit tests cannot.

## CI Integration Tests

The integration tests in `tests/network/` now run in CI and verify:

### Restricted Mode Tests
- ✅ `network_restricted_allows_internet.py` - Can reach public internet
- ✅ `network_restricted_blocks_private.py` - RFC1918 networks blocked
- ✅ `network_restricted_blocks_local_gateway.py` - Local gateway blocked
- ✅ `network_restricted_blocks_metadata.py` - Metadata endpoints blocked

### Allowlist Mode Tests
- ✅ `test_allowlist.py::test_allowlist_mode_allows_specified_domains` - Allowed domains work
- ✅ `test_allowlist.py::test_allowlist_blocks_non_allowed_domains` - Non-allowed blocked
- ✅ `test_allowlist.py::test_allowlist_always_blocks_rfc1918` - RFC1918 always blocked

### Open Mode Tests
- ✅ `network_open_allows_all.py` - No restrictions applied

## Test Strategy

1. **Unit tests** ✅ - Fast, catch logic bugs (rule ordering, deduplication)
2. **Integration tests** ✅ - Run in CI, catch connectivity issues
3. **Manual testing** ✅ - For complex scenarios and debugging

This comprehensive approach ensures both the logic and the actual network behavior are correct.

## Test Coverage Summary

| What | Where | Runs |
|------|-------|------|
| ACL rule generation | `internal/network/acl_test.go` | Every commit (unit) |
| Rule ordering | `internal/network/acl_test.go` | Every commit (unit) |
| IP deduplication | `internal/network/acl_test.go` | Every commit (unit) |
| Config parsing | `internal/config/config_test.go` | Every commit (unit) |
| DNS resolution | `internal/network/resolver_test.go` | Every commit (unit) |
| Restricted mode | `tests/network/network_restricted_*.py` | Every commit (integration) |
| Allowlist mode | `tests/network/test_allowlist.py` | Every commit (integration) |
| Open mode | `tests/network/network_open_*.py` | Every commit (integration) |

## Conclusion

The combination of **unit tests** (logic) and **integration tests** (connectivity) provides comprehensive coverage:

- Unit tests catch logic bugs (rule ordering, deduplication) - **Fast**
- Integration tests catch connectivity bugs (gateway routing, DNS) - **Thorough**

Both run on every commit in CI, ensuring high confidence in the network isolation implementation.
