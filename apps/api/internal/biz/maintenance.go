package biz

import "sync/atomic"

// Maintenance is a process-global flag indicating that Dockery is in a
// read-only / no-mutation window — currently used only to gate UI-side
// writes during a garbage-collect run. The registry process itself is
// stopped during GC, so docker CLI pushes / deletes will fail naturally
// with a connection error; this flag is the UI's early-rejection path
// so operators see a clean 503 instead of a confusing upstream error.
type Maintenance struct {
	active atomic.Bool
}

// NewMaintenance returns a fresh flag in the "not in maintenance" state.
// It's a wire provider; exactly one instance is shared across the biz
// layer and the registry proxy.
func NewMaintenance() *Maintenance { return &Maintenance{} }

// Enter marks maintenance as active. Safe to call while already active.
func (m *Maintenance) Enter() { m.active.Store(true) }

// Exit clears the flag. Safe to call when not active.
func (m *Maintenance) Exit() { m.active.Store(false) }

// Active reports whether maintenance is currently in progress.
func (m *Maintenance) Active() bool { return m.active.Load() }
