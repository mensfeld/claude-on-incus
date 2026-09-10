package monitor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mensfeld/code-on-incus/internal/container"
	"github.com/mensfeld/code-on-incus/internal/network"
)

// Responder handles automated responses to threats
type Responder struct {
	containerName      string
	autoPauseOnHigh    bool
	autoKillOnCritical bool
	forensicsOnKill    bool // preserve (rename) the container for forensics instead of deleting it (default true)
	auditLog           *AuditLog
	onThreat           func(ThreatEvent)
	onAction           func(action, message string) // Called when container is paused/killed
	onError            func(error)                  // Called for non-fatal cleanup errors (routes to the session log, never the terminal)

	// State tracking to prevent infinite loops
	mu            sync.Mutex
	paused        bool
	killed        bool
	recentThreats map[string]time.Time // threat key -> last alert time
	dedupeWindow  time.Duration
}

// NewResponder creates a new threat responder
func NewResponder(containerName string, autoPauseOnHigh, autoKillOnCritical bool,
	auditLog *AuditLog, onThreat func(ThreatEvent),
) *Responder {
	return &Responder{
		containerName:      containerName,
		autoPauseOnHigh:    autoPauseOnHigh,
		autoKillOnCritical: autoKillOnCritical,
		forensicsOnKill:    true, // default on; see SetForensicsOnKill
		auditLog:           auditLog,
		onThreat:           onThreat,
		recentThreats:      make(map[string]time.Time),
		dedupeWindow:       30 * time.Second, // Don't re-alert for same threat within 30s
	}
}

// SetForensicsOnKill controls whether killContainer preserves the container
// for forensics (renamed) instead of deleting it ([monitoring] forensics_on_kill,
// default true).
func (r *Responder) SetForensicsOnKill(enabled bool) {
	r.forensicsOnKill = enabled
}

// SetOnAction sets a callback for when critical actions (pause/kill) are taken
func (r *Responder) SetOnAction(callback func(action, message string)) {
	r.onAction = callback
}

// SetOnError sets a callback for non-fatal cleanup errors. The daemons wire this
// to the session logger so the warnings are recorded in
// ~/.coi/logs/<container>.stderr.log instead of being written to the user's
// attached terminal (issue #372 class).
func (r *Responder) SetOnError(callback func(error)) {
	r.onError = callback
}

// reportError routes a non-fatal error to the onError callback when set. It is
// deliberately silent when no callback is set: these are best-effort cleanup
// warnings on the kill path and must never fall back to a terminal sink.
func (r *Responder) reportError(err error) {
	if r.onError != nil {
		r.onError(err)
	}
}

// Handle processes a threat and takes appropriate action
func (r *Responder) Handle(ctx context.Context, threat ThreatEvent) error {
	r.mu.Lock()

	// If already killed, nothing more to do
	if r.killed {
		r.mu.Unlock()
		return nil
	}

	// Deduplicate recent threats - create a key from threat category and title
	threatKey := threat.Category + ":" + threat.Title
	if s := threat.Evidence.String(); s != "" {
		// Include evidence summary in key for more precise deduplication
		threatKey += ":" + s
	}

	now := time.Now()
	if lastSeen, exists := r.recentThreats[threatKey]; exists {
		if now.Sub(lastSeen) < r.dedupeWindow {
			// Already alerted for this threat recently, just log silently
			r.mu.Unlock()
			threat.Action = "deduplicated"
			return r.logThreat(threat)
		}
	}
	r.recentThreats[threatKey] = now

	// Clean up old entries from the map periodically
	if len(r.recentThreats) > 100 {
		for key, ts := range r.recentThreats {
			if now.Sub(ts) > r.dedupeWindow*2 {
				delete(r.recentThreats, key)
			}
		}
	}

	// Check if already paused (for high-level threats that would pause)
	alreadyPaused := r.paused
	r.mu.Unlock()

	// Determine action based on threat level
	switch threat.Level {
	case ThreatLevelInfo:
		threat.Action = "logged"
		return r.logThreat(threat)

	case ThreatLevelWarning:
		threat.Action = "alerted"
		r.alert(threat)
		return r.logThreat(threat)

	case ThreatLevelHigh:
		if r.autoPauseOnHigh {
			if alreadyPaused {
				// Already paused, just log
				threat.Action = "logged (already paused)"
				return r.logThreat(threat)
			}
			threat.Action = "paused"
			r.alert(threat)
			if err := r.logThreat(threat); err != nil {
				return err
			}
			return r.pauseContainer(ctx)
		}
		threat.Action = "alerted"
		r.alert(threat)
		return r.logThreat(threat)

	case ThreatLevelCritical:
		if r.autoKillOnCritical {
			threat.Action = "killed"
			r.alert(threat)
			if err := r.logThreat(threat); err != nil {
				return err
			}
			return r.killContainer(ctx)
		}
		threat.Action = "alerted"
		r.alert(threat)
		return r.logThreat(threat)
	}

	return nil
}

// logThreat writes threat to audit log
func (r *Responder) logThreat(threat ThreatEvent) error {
	if r.auditLog != nil {
		return r.auditLog.WriteThreat(threat)
	}
	return nil
}

// alert notifies via callback
func (r *Responder) alert(threat ThreatEvent) {
	if r.onThreat != nil {
		r.onThreat(threat)
	}
}

// pauseContainer pauses the container
func (r *Responder) pauseContainer(ctx context.Context) error {
	r.mu.Lock()
	if r.paused {
		r.mu.Unlock()
		return nil // Already paused
	}
	r.mu.Unlock()

	// Use IncusOutputWithStderr to capture error messages from Incus
	// (like "already frozen" which goes to stderr)
	output, err := container.IncusOutputWithStderrContext(ctx, "pause", r.containerName)
	if err != nil {
		// Check if error is because container is already paused
		// Incus returns "The container is already frozen" for this case
		// The message may be in err.Error() or in the combined output
		errStr := err.Error() + " " + output
		if strings.Contains(errStr, "already frozen") ||
			strings.Contains(errStr, "already paused") {
			r.mu.Lock()
			r.paused = true
			r.mu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to pause container: %w", err)
	}

	r.mu.Lock()
	r.paused = true
	r.mu.Unlock()

	// Notify about the pause action
	if r.onAction != nil {
		r.onAction("paused", fmt.Sprintf("Container %s PAUSED due to security threat. Unfreeze with: coi unfreeze %s", r.containerName, r.containerName))
	}

	return nil
}

// killContainer stops and removes the container from service, preserving it
// for forensics (unless disabled): the auto-kill fires exactly when the
// container's state is most worth investigating — deleting it with the threat
// would destroy the evidence of HOW the attempt worked, leaving only the
// audit log ("snapshot state for investigation before deactivating", Trail of
// Bits). Either way the container is GONE under its original name.
func (r *Responder) killContainer(callerCtx context.Context) error {
	r.mu.Lock()
	if r.killed {
		r.mu.Unlock()
		return nil // Already killed
	}
	r.mu.Unlock()

	// DETACHED context for the destructive steps. The caller's context is the
	// monitoring daemon's — and stopping the container ENDS the attached
	// session, which tears the daemon down and cancels that context. Using it
	// here SIGKILLs the very `incus stop`/`rename`/`delete` mid-run (surfacing
	// as "exit status -1"), the kill's own action pulling the rug from under
	// it. A fresh timeout-bounded context is immune to that self-cancellation;
	// the timeout still bounds a genuinely stuck Incus. (If the caller already
	// cancelled — real shutdown — honor that before we start.)
	if callerCtx.Err() != nil {
		return callerCtx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), killOperationTimeout)
	defer cancel()

	// Forensic preservation is ZERO-COPY: clear the ephemeral flag while the
	// container is still running (so the stop below cannot auto-delete it),
	// then RENAME the stopped container to the forensics name instead of
	// deleting it. An `incus copy` would transfer the whole rootfs — tens of
	// seconds on CI-class pools, delaying the security response — while the
	// property flip + rename are instant and preserve the actual instance,
	// not a copy. Best-effort: any failure falls back to the plain
	// stop-and-delete kill.
	forensicName := ""
	if r.forensicsOnKill {
		r.pruneForensicCopies(ctx)
		if _, err := container.IncusOutputContext(ctx, "config", "set", r.containerName, "ephemeral=false", "--property"); err != nil {
			r.reportError(fmt.Errorf("failed to clear ephemeral flag for forensics (continuing with plain kill): %w", err))
		} else {
			forensicName = forensicCopyName(r.containerName, time.Now())
		}
	}
	forensicNote := ""
	if forensicName != "" {
		forensicNote = fmt.Sprintf(" — container preserved for forensics as %q (inspect with `incus file pull`/`incus start`, dispose with `incus delete`)", forensicName)
	}

	// Notify about the kill action BEFORE killing
	if r.onAction != nil {
		r.onAction("killed", fmt.Sprintf("Container %s KILLED due to critical security threat%s", r.containerName, forensicNote))
	}

	// Get container IP BEFORE stopping (needed for cleanup)
	containerIP, _ := network.GetContainerIPFast(r.containerName)

	// Stop the container. Best-effort: a nonzero stop does NOT abort the
	// response — the container may already be stopping (the session teardown
	// races us) or the forkstart soft-fail returns nonzero though the stop
	// took effect. We proceed to preserve/delete regardless; the terminal
	// state, not this exit code, is what matters.
	if _, err := container.StopContainerQuiet(ctx, r.containerName, true); err != nil {
		r.reportError(fmt.Errorf("stop during kill returned an error (continuing): %w", err))
	}

	// Preserve (rename) IMMEDIATELY after the stop, before the firewall
	// cleanups below: stopping the container also ends the attached session,
	// whose own teardown then races to delete the (now stopped, no longer
	// ephemeral) container — every millisecond between stop and rename widens
	// that window. Once renamed, the teardown's delete of the ORIGINAL name is
	// a harmless "not found". The rename of a stopped container is instant; a
	// failed rename falls back to force-delete so the kill's contract (the
	// container is GONE under its name) always holds.
	if forensicName != "" {
		if _, renameErr := container.IncusOutputContext(ctx, "rename", r.containerName, forensicName); renameErr != nil {
			r.reportError(fmt.Errorf("failed to preserve forensic container (deleting instead): %w", renameErr))
			forensicName = ""
		}
	}

	// Clean up firewall and NFT monitoring rules (keyed on the captured IP,
	// not the container name, so the rename above does not affect them).
	if containerIP != "" {
		if err := r.cleanupNftRules(containerIP); err != nil {
			// Log warning but don't fail the kill operation
			r.reportError(fmt.Errorf("failed to cleanup nft rules: %w", err))
		}
		if err := r.cleanupNFTRules(containerIP); err != nil {
			// Log warning but don't fail the kill operation
			r.reportError(fmt.Errorf("failed to cleanup NFT monitoring rules: %w", err))
		}
	}

	if forensicName == "" {
		// --force so a still-running container (a failed stop above) is still
		// removed; tolerate "not found" — for an ephemeral container the
		// session teardown may have deleted it already, and the kill's goal
		// state (gone under its name) is reached either way.
		if out, delErr := container.IncusOutputWithStderrContext(ctx, "delete", "--force", r.containerName); delErr != nil {
			if !container.IsNotFoundErr(delErr) && !strings.Contains(strings.ToLower(out), "not found") {
				return fmt.Errorf("failed to delete container: %w", delErr)
			}
		}
	}

	r.mu.Lock()
	r.killed = true
	r.mu.Unlock()
	return nil
}

// killOperationTimeout bounds the detached kill sequence so a genuinely stuck
// Incus daemon cannot hang the responder goroutine forever.
const killOperationTimeout = 90 * time.Second

// maxForensicCopies caps how many forensic copies may exist per container
// name (oldest pruned first), so repeated incidents on the same slot cannot
// fill the storage pool.
const maxForensicCopies = 3

// forensicCopyName derives the preserved container's name. The unix-seconds suffix
// is fixed-width for the next few centuries, so lexicographic order equals
// chronological order — pruning can sort names directly.
func forensicCopyName(containerName string, now time.Time) string {
	return fmt.Sprintf("%s-forensics-%d", containerName, now.Unix())
}

// forensicCopiesToPrune returns the names to delete so that, together with the
// one copy about to be created, at most maxForensicCopies remain. names may
// contain unrelated containers; only "<containerName>-forensics-" entries are
// considered. Oldest (lexicographically smallest suffix) go first.
func forensicCopiesToPrune(names []string, containerName string) []string {
	prefix := containerName + "-forensics-"
	var copies []string
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			copies = append(copies, n)
		}
	}
	sort.Strings(copies)
	// Keep maxForensicCopies-1 existing so the new copy fits under the cap.
	if excess := len(copies) - (maxForensicCopies - 1); excess > 0 {
		return copies[:excess]
	}
	return nil
}

// pruneForensicCopies deletes the oldest preserved containers beyond the cap
// so the one about to be created fits under it. Best-effort: a failed
// list/delete must not stop the evidence preservation or the kill.
func (r *Responder) pruneForensicCopies(ctx context.Context) {
	out, err := container.IncusOutputContext(ctx, "list", "--format", "csv", "-c", "n", r.containerName+"-forensics-")
	if err != nil {
		return
	}
	names := strings.Fields(strings.TrimSpace(out))
	for _, stale := range forensicCopiesToPrune(names, r.containerName) {
		if _, err := container.IncusOutputContext(ctx, "delete", "--force", stale); err != nil {
			r.reportError(fmt.Errorf("failed to prune old forensic container %s: %w", stale, err))
		}
	}
}

// cleanupNftRules removes nft rules for a container IP
func (r *Responder) cleanupNftRules(containerIP string) error {
	fm := network.NewNftManager(containerIP, "")
	return fm.RemoveRules()
}

// cleanupNFTRules removes NFT monitoring rules for a container IP
func (r *Responder) cleanupNFTRules(containerIP string) error {
	return network.CleanupNFTMonitoringRules(containerIP)
}
