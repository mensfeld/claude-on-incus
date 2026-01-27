# COI Health & Auto-Configuration

This document outlines planned improvements for automatic network configuration and health diagnostics.

## Issue: OVN Host Routing

### Problem
When using OVN networks, containers are on an isolated subnet (e.g., 10.215.220.0/24) that's separate from the host. Users trying to access container services (web servers, databases, etc.) from their host machine experience connection timeouts because the host doesn't know how to route to the OVN subnet.

**Example:**
```bash
# User starts Rails server in container on OVN network
coi shell --network=restricted
> rails server -b 0.0.0.0

# From host browser/curl - FAILS with timeout
curl http://10.215.220.2:3000  # ❌ Connection timeout
```

### Root Cause
OVN creates an isolated virtual network that routes through the uplink bridge (incusbr0) as a gateway. The host needs an explicit route to reach the OVN subnet:

```bash
# Missing route
sudo ip route add 10.215.220.0/24 via 10.47.62.1 dev incusbr0
```

### Solution: Auto-Add Host Route

**Implementation Plan:**

1. **Detection in `network/manager.go`:**
   - In `SetupForContainer()`, detect if container uses OVN network
   - Check existing routes: `ip route show | grep <ovn_subnet>`
   - If route missing, attempt to add it automatically

2. **New function `ensureHostRoute()`:**
   ```go
   // ensureHostRoute ensures the host can route to the OVN network subnet
   func ensureHostRoute(networkName string) error {
       // 1. Check if network is OVN type
       isOVN, err := isOVNNetwork(networkName)
       if err != nil || !isOVN {
           return nil // Skip for non-OVN networks
       }

       // 2. Get OVN subnet from network config (ipv4.address)
       subnet, err := getNetworkSubnet(networkName)
       if err != nil {
           return err
       }

       // 3. Get uplink network and its gateway IP
       uplinkName, err := getOVNUplink(networkName)
       if err != nil {
           return err
       }
       gatewayIP, err := getNetworkGateway(uplinkName)
       if err != nil {
           return err
       }

       // 4. Check if route already exists
       if routeExists(subnet, gatewayIP) {
           return nil // Already configured
       }

       // 5. Try to add route (requires sudo/root)
       cmd := exec.Command("ip", "route", "add", subnet, "via", gatewayIP, "dev", uplinkName)
       if err := cmd.Run(); err != nil {
           // Return instructional error if we can't add route
           return fmt.Errorf("could not add host route (need sudo): ip route add %s via %s dev %s",
               subnet, gatewayIP, uplinkName)
       }

       log.Printf("Added host route: %s via %s (dev %s)", subnet, gatewayIP, uplinkName)
       return nil
   }
   ```

3. **Helper functions:**
   ```go
   func isOVNNetwork(name string) (bool, error)
   func getNetworkSubnet(name string) (string, error)  // Returns "10.215.220.0/24"
   func getOVNUplink(name string) (string, error)      // Returns "incusbr0"
   func getNetworkGateway(name string) (string, error) // Returns "10.47.62.1"
   func routeExists(subnet, gateway string) bool
   ```

4. **User Experience:**
   ```
   Starting container on OVN network (ovn-net)...
   ✓ Host route configured: 10.215.220.0/24 via 10.47.62.1

   Container services accessible at: http://10.215.220.2:PORT
   ```

   Or if sudo fails:
   ```
   Starting container on OVN network (ovn-net)...
   ⚠️  Host route missing - container services won't be accessible from host

   To access services running in the container from your host:
     sudo ip route add 10.215.220.0/24 via 10.47.62.1 dev incusbr0

   Container: coi-workspace-1
   ```

5. **Cleanup in `Teardown()`:**
   - Optionally remove the route when last COI container using that OVN network stops
   - Check if other COI containers still use the network before removing route
   - Log route removal: "Removed host route: 10.215.220.0/24"

**Files to Modify:**
- `internal/network/manager.go` - Add `ensureHostRoute()` and helpers
- `internal/network/manager.go` - Call `ensureHostRoute()` in `SetupForContainer()`
- `internal/network/manager.go` - Add route cleanup logic in `Teardown()`

**Testing:**
- Add integration test for host-to-container connectivity on OVN
- Test with/without sudo access
- Test cleanup when last container stops
- Test multiple containers on same OVN network (don't re-add route)

---

## Feature: `coi health` Command

### Purpose
Provide diagnostic and automatic fix capabilities for common COI setup issues.

### Use Cases

1. **New user setup validation**
   ```bash
   $ coi health
   Running COI health checks...

   ✓ Incus installed (version 6.7)
   ✓ User in incus-admin group
   ✓ Incus storage pool configured (default: btrfs)
   ✓ Incus network configured (incusbr0: 10.47.62.0/24)
   ✓ OVN network detected (ovn-net: 10.215.220.0/24)
   ✓ Host can route to OVN network
   ✓ Container base image available (coi)

   All checks passed! ✓
   ```

2. **Problem detection**
   ```bash
   $ coi health
   Running COI health checks...

   ✓ Incus installed (version 6.7)
   ✗ User NOT in incus-admin group
     Fix: sudo usermod -aG incus-admin $USER (requires re-login)

   ✓ Incus storage pool configured
   ⚠️ Incus network has DNS issues (127.0.0.53 stub resolver)
     This can cause build failures. COI will auto-fix during builds.

   ✓ OVN network detected (ovn-net)
   ✗ Host cannot route to OVN network
     Fix: sudo ip route add 10.215.220.0/24 via 10.47.62.1 dev incusbr0

   ⚠️ COI base image not found - will be built on first use

   2 errors, 1 warning found.
   ```

3. **Auto-fix mode**
   ```bash
   $ coi health --fix
   Running COI health checks with auto-fix...

   ✓ Incus installed
   ✗ Host route missing for OVN network
     → Adding route: sudo ip route add 10.215.220.0/24 via 10.47.62.1
     ✓ Route added successfully

   ⚠️ COI base image not found
     → Building COI base image (this may take a few minutes)...
     ✓ Image built successfully

   All fixable issues resolved! ✓
   ```

### Health Checks to Implement

#### 1. System Requirements
- [ ] **Incus installed** - Check `incus version` works
- [ ] **Incus version** - Warn if < 6.0
- [ ] **User permissions** - Check if user in `incus-admin` group
- [ ] **Kernel features** - Check idmap support (`/proc/sys/kernel/unprivileged_userns_clone`)

#### 2. Incus Configuration
- [ ] **Storage pool** - Check `incus storage list` has at least one pool
- [ ] **Network** - Check default profile has network device
- [ ] **Network type** - Detect bridge vs OVN
- [ ] **DNS configuration** - Check if systemd-resolved stub resolver (127.0.0.53) is configured

#### 3. OVN-Specific (if OVN detected)
- [ ] **OVN packages** - Check `ovn-host` and `ovn-central` installed
- [ ] **OVN network** - Check if OVN network configured in Incus
- [ ] **Uplink network** - Verify OVN uplink bridge exists and has proper config
- [ ] **Host routing** - Check `ip route` has route to OVN subnet
- [ ] **Security.acls support** - Verify network supports ACLs (for restricted/allowlist modes)

#### 4. COI-Specific
- [ ] **Base image** - Check if `coi` image exists
- [ ] **Running containers** - List active COI containers
- [ ] **Stale containers** - Detect stopped COI containers that could be cleaned
- [ ] **Config file** - Check `~/.config/coi/config.toml` syntax if exists
- [ ] **Cache directory** - Check `~/.cache/coi/` exists and is writable

#### 5. Network Connectivity (in-container)
- [ ] **Internet access** - Spawn test container, ping 8.8.8.8
- [ ] **DNS resolution** - Test `getent hosts google.com` in container
- [ ] **Package install** - Test `apt update` works (DNS + HTTP)

### Implementation Plan

**New file:** `cmd/health.go`
```go
package cmd

type HealthCheck struct {
    Name        string
    Category    string
    Required    bool  // false = warning only
    CheckFunc   func() HealthResult
    FixFunc     func() error  // nil if not auto-fixable
}

type HealthResult struct {
    Status      HealthStatus  // Pass, Fail, Warning
    Message     string
    FixCommand  string       // Optional command user can run
}

type HealthStatus int
const (
    HealthPass HealthStatus = iota
    HealthWarning
    HealthFail
)

func runHealthChecks(autoFix bool) error {
    checks := []HealthCheck{
        {Name: "Incus installed", CheckFunc: checkIncusInstalled},
        {Name: "User permissions", CheckFunc: checkUserPerms, FixFunc: nil},
        {Name: "Storage configured", CheckFunc: checkStorage},
        {Name: "Network configured", CheckFunc: checkNetwork},
        {Name: "OVN host route", CheckFunc: checkOVNRoute, FixFunc: fixOVNRoute},
        {Name: "Base image", CheckFunc: checkBaseImage, FixFunc: buildBaseImage},
        // ... more checks
    }

    results := runChecks(checks, autoFix)
    printResults(results)
    return nil
}
```

**New file:** `internal/health/checks.go`
- Implement individual check functions
- Use existing `internal/container` and `internal/network` packages

**New file:** `internal/health/fixes.go`
- Implement auto-fix functions
- Handle sudo prompts gracefully
- Return detailed errors for manual fixes

**CLI Integration:**
```go
// In cmd/root.go
var healthCmd = &cobra.Command{
    Use:   "health",
    Short: "Check COI setup and diagnose issues",
    Long:  `Runs diagnostic checks on COI configuration...`,
    RunE:  runHealth,
}

var healthFix bool
func init() {
    rootCmd.AddCommand(healthCmd)
    healthCmd.Flags().BoolVar(&healthFix, "fix", false, "Automatically fix issues where possible")
}
```

**Output Format:**
```
COI Health Check v0.4.0
========================

System Requirements
  ✓ Incus installed (v6.7)
  ✓ User permissions (incus-admin group)
  ✓ Kernel features (idmap support)

Incus Configuration
  ✓ Storage pool (default: btrfs, 15GB)
  ✓ Network (incusbr0: bridge)
  ⚠️ DNS configuration (systemd-resolved stub)
     └─ Auto-fixed during container builds

OVN Networking
  ✓ OVN packages installed
  ✓ OVN network configured (ovn-net)
  ✗ Host routing missing
     └─ Fix: sudo ip route add 10.215.220.0/24 via 10.47.62.1 dev incusbr0

COI Setup
  ✓ Base image (coi:latest)
  ✓ Config file (~/.config/coi/config.toml)
  ⚠️ 2 stopped containers found (run 'coi clean' to remove)

Network Connectivity
  ✓ Internet access (8.8.8.8 reachable)
  ✓ DNS resolution (google.com)
  ✓ Package manager (apt update works)

Summary: 1 error, 2 warnings
Run 'coi health --fix' to automatically fix issues.
```

### Priority Order
1. Implement auto-route in `network/manager.go` (blocks user workflows)
2. Implement basic `coi health` command (most valuable checks first)
3. Add auto-fix capabilities to health checks
4. Expand health checks based on user feedback

### Testing
- Add integration tests for each health check
- Test `--fix` mode with and without sudo
- Test on clean system (no Incus installed)
- Test on misconfigured system (broken DNS, missing routes)

---

## Related Issues

- **CI Issue**: GitHub Actions runners needed manual route configuration for OVN
- **User Issue #83**: (check if related to these improvements)

## Implementation Timeline

**Phase 1: Critical Fix** (Current PR)
- [x] Document the issue and solution (this file)
- [ ] Wait for CI to pass
- [ ] Merge OVN network isolation PR

**Phase 2: Auto-Routing** (Next PR)
- [ ] Implement `ensureHostRoute()` in network manager
- [ ] Add integration tests
- [ ] Update README with OVN setup documentation

**Phase 3: Health Command** (Future PR)
- [ ] Implement basic `coi health` command
- [ ] Add auto-fix for route configuration
- [ ] Add auto-fix for base image
- [ ] Expand health checks based on testing

**Phase 4: Polish** (Future)
- [ ] Add `coi health --json` for scripting
- [ ] Add `coi doctor` alias (common convention)
- [ ] Integration with `coi shell` (run health check if container fails to start)
