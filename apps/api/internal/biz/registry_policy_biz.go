package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"api/internal/data/ent"
	"api/internal/data/ent/systemsetting"

	"golang.org/x/sync/semaphore"
)

var (
	ErrRegistryPolicyNotInitialized = errors.New("registry policy not initialized")
	ErrRegistryPolicyConflict       = errors.New("registry policy version conflict")
	ErrRegistryPolicyInvalid        = errors.New("invalid registry policy")
)

// RegistryPolicy is the strongly typed registry-wide runtime policy decoded
// from its entries in the shared system_settings table.
type RegistryPolicy struct {
	PreventTagOverwrite bool
	OverwriteExclusions []string
	Version             int64
	UpdatedAt           time.Time
	UpdatedBy           string
}

const (
	SettingKeyPreventTagOverwrite   = "registry.prevent_tag_overwrite"
	SettingKeyOverwriteExclusions   = "registry.tag_overwrite_exclusions"
	registryPolicyGateCapacity      = int64(1 << 30)
	maxTagOverwriteExclusionEntries = 128
)

var registryTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// RegistryPolicyBiz owns the in-memory policy snapshot and the switch
// barrier. Manifest PUTs hold one gate permit for their full lifetime;
// updates acquire the entire gate, so a successful update cleanly separates
// requests using the old policy from requests using the new one.
type RegistryPolicyBiz struct {
	db       *ent.Client
	gate     *semaphore.Weighted
	snapshot atomic.Pointer[RegistryPolicy]
}

func NewRegistryPolicyBiz(db *ent.Client) *RegistryPolicyBiz {
	return &RegistryPolicyBiz{
		db:   db,
		gate: semaphore.NewWeighted(registryPolicyGateCapacity),
	}
}

// Initialize loads the persisted values before the HTTP server starts.
// Missing keys are represented by a virtual version-0 default and are not
// written: system_settings only records administrator changes.
func (u *RegistryPolicyBiz) Initialize(ctx context.Context) error {
	p, err := u.loadPolicy(ctx)
	if err != nil {
		return fmt.Errorf("initialize registry policy: %w", err)
	}
	u.snapshot.Store(cloneRegistryPolicy(p))
	return nil
}

func (u *RegistryPolicyBiz) Get() (*RegistryPolicy, error) {
	p := u.snapshot.Load()
	if p == nil {
		return nil, ErrRegistryPolicyNotInitialized
	}
	return cloneRegistryPolicy(p), nil
}

// EnterManifestPut returns a stable policy snapshot and a release function.
// Callers must defer release until the upstream manifest PUT has completed.
func (u *RegistryPolicyBiz) EnterManifestPut(ctx context.Context) (*RegistryPolicy, func(), error) {
	if err := u.gate.Acquire(ctx, 1); err != nil {
		return nil, nil, err
	}
	p := u.snapshot.Load()
	if p == nil {
		u.gate.Release(1)
		return nil, nil, ErrRegistryPolicyNotInitialized
	}
	return cloneRegistryPolicy(p), func() { u.gate.Release(1) }, nil
}

// IsOverwriteExcluded reports whether an exact tag name is allowed to move
// even while overwrite protection is enabled.
func (p *RegistryPolicy) IsOverwriteExcluded(tag string) bool {
	for _, excluded := range p.OverwriteExclusions {
		if tag == excluded {
			return true
		}
	}
	return false
}

// Update changes the policy with optimistic concurrency control. Acquiring
// the entire gate makes the update a linearization point for manifest PUTs.
func (u *RegistryPolicyBiz) Update(
	ctx context.Context,
	expectedVersion int64,
	preventTagOverwrite bool,
	overwriteExclusions []string,
	actor string,
) (*RegistryPolicy, error) {
	if expectedVersion < 0 {
		return nil, fmt.Errorf("%w: expected version must not be negative", ErrRegistryPolicyInvalid)
	}
	if actor == "" {
		return nil, fmt.Errorf("%w: actor is required", ErrRegistryPolicyInvalid)
	}
	normalizedExclusions, err := normalizeOverwriteExclusions(overwriteExclusions)
	if err != nil {
		return nil, err
	}
	if err := u.gate.Acquire(ctx, registryPolicyGateCapacity); err != nil {
		return nil, err
	}
	defer u.gate.Release(registryPolicyGateCapacity)

	current := u.snapshot.Load()
	if current == nil {
		return nil, ErrRegistryPolicyNotInitialized
	}
	if current.Version != expectedVersion {
		return cloneRegistryPolicy(current), ErrRegistryPolicyConflict
	}
	if current.PreventTagOverwrite == preventTagOverwrite &&
		slices.Equal(current.OverwriteExclusions, normalizedExclusions) {
		return cloneRegistryPolicy(current), nil
	}

	updated, err := u.updateSettings(
		ctx,
		expectedVersion,
		preventTagOverwrite,
		normalizedExclusions,
		actor,
	)
	if err != nil {
		if errors.Is(err, ErrRegistryPolicyConflict) {
			if latest, loadErr := u.loadPolicy(ctx); loadErr == nil {
				u.snapshot.Store(cloneRegistryPolicy(latest))
				return cloneRegistryPolicy(latest), ErrRegistryPolicyConflict
			}
		}
		return nil, err
	}
	u.snapshot.Store(cloneRegistryPolicy(updated))
	return cloneRegistryPolicy(updated), nil
}

func (u *RegistryPolicyBiz) loadPolicy(ctx context.Context) (*RegistryPolicy, error) {
	settings, err := u.db.SystemSetting.Query().
		Where(systemsetting.KeyIn(
			SettingKeyPreventTagOverwrite,
			SettingKeyOverwriteExclusions,
		)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var overwriteSetting, exclusionsSetting *ent.SystemSetting
	for _, setting := range settings {
		switch setting.Key {
		case SettingKeyPreventTagOverwrite:
			overwriteSetting = setting
		case SettingKeyOverwriteExclusions:
			exclusionsSetting = setting
		}
	}
	if overwriteSetting == nil {
		if exclusionsSetting != nil {
			return nil, fmt.Errorf("registry policy invalid: exclusions exist without the primary setting")
		}
		return &RegistryPolicy{
			PreventTagOverwrite: false,
			OverwriteExclusions: []string{},
			Version:             0,
			UpdatedBy:           "system",
		}, nil
	}

	p, err := registryPolicyFromSetting(overwriteSetting)
	if err != nil {
		return nil, err
	}
	if exclusionsSetting != nil {
		p.OverwriteExclusions, err = overwriteExclusionsFromSetting(exclusionsSetting)
		if err != nil {
			return nil, err
		}
	}
	if err := validateRegistryPolicy(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (u *RegistryPolicyBiz) updateSettings(
	ctx context.Context,
	expectedVersion int64,
	preventTagOverwrite bool,
	overwriteExclusions []string,
	actor string,
) (*RegistryPolicy, error) {
	overwriteValue, err := json.Marshal(preventTagOverwrite)
	if err != nil {
		return nil, fmt.Errorf("marshal registry overwrite policy: %w", err)
	}
	exclusionsValue, err := json.Marshal(overwriteExclusions)
	if err != nil {
		return nil, fmt.Errorf("marshal registry overwrite exclusions: %w", err)
	}

	tx, err := u.db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	newVersion := expectedVersion + 1
	var primary *ent.SystemSetting
	if expectedVersion == 0 {
		primary, err = tx.SystemSetting.Create().
			SetKey(SettingKeyPreventTagOverwrite).
			SetValue(overwriteValue).
			SetVersion(newVersion).
			SetUpdatedBy(actor).
			Save(ctx)
		if ent.IsConstraintError(err) {
			return nil, ErrRegistryPolicyConflict
		}
		if err != nil {
			return nil, err
		}
	} else {
		updated, updateErr := tx.SystemSetting.Update().
			Where(
				systemsetting.KeyEQ(SettingKeyPreventTagOverwrite),
				systemsetting.VersionEQ(expectedVersion),
			).
			SetValue(overwriteValue).
			SetVersion(newVersion).
			SetUpdatedBy(actor).
			Save(ctx)
		if updateErr != nil {
			return nil, updateErr
		}
		if updated == 0 {
			return nil, ErrRegistryPolicyConflict
		}
		primary, err = tx.SystemSetting.Query().
			Where(systemsetting.KeyEQ(SettingKeyPreventTagOverwrite)).
			Only(ctx)
		if err != nil {
			return nil, err
		}
	}

	exclusionsSetting, queryErr := tx.SystemSetting.Query().
		Where(systemsetting.KeyEQ(SettingKeyOverwriteExclusions)).
		Only(ctx)
	switch {
	case ent.IsNotFound(queryErr) && len(overwriteExclusions) == 0:
		// Preserve the missing-key default until an administrator configures
		// at least one exception.
	case ent.IsNotFound(queryErr):
		_, err = tx.SystemSetting.Create().
			SetKey(SettingKeyOverwriteExclusions).
			SetValue(exclusionsValue).
			SetVersion(newVersion).
			SetUpdatedBy(actor).
			Save(ctx)
	case queryErr != nil:
		return nil, queryErr
	default:
		_, err = tx.SystemSetting.UpdateOne(exclusionsSetting).
			SetValue(exclusionsValue).
			SetVersion(newVersion).
			SetUpdatedBy(actor).
			Save(ctx)
	}
	if ent.IsConstraintError(err) {
		return nil, ErrRegistryPolicyConflict
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	return &RegistryPolicy{
		PreventTagOverwrite: preventTagOverwrite,
		OverwriteExclusions: slices.Clone(overwriteExclusions),
		Version:             newVersion,
		UpdatedAt:           primary.UpdatedAt,
		UpdatedBy:           actor,
	}, nil
}

func registryPolicyFromSetting(setting *ent.SystemSetting) (*RegistryPolicy, error) {
	if setting == nil {
		return nil, fmt.Errorf("registry policy setting is nil")
	}
	if setting.Key != SettingKeyPreventTagOverwrite {
		return nil, fmt.Errorf("unexpected registry policy key %q", setting.Key)
	}
	var decoded any
	if err := json.Unmarshal(setting.Value, &decoded); err != nil {
		return nil, fmt.Errorf("registry policy invalid: %s must be a JSON boolean: %w", setting.Key, err)
	}
	enabled, ok := decoded.(bool)
	if !ok {
		return nil, fmt.Errorf("registry policy invalid: %s must be a JSON boolean", setting.Key)
	}
	return &RegistryPolicy{
		PreventTagOverwrite: enabled,
		OverwriteExclusions: []string{},
		Version:             setting.Version,
		UpdatedAt:           setting.UpdatedAt,
		UpdatedBy:           setting.UpdatedBy,
	}, nil
}

func overwriteExclusionsFromSetting(setting *ent.SystemSetting) ([]string, error) {
	if setting == nil || setting.Key != SettingKeyOverwriteExclusions {
		return nil, fmt.Errorf("unexpected registry overwrite exclusions setting")
	}
	var exclusions []string
	if err := json.Unmarshal(setting.Value, &exclusions); err != nil {
		return nil, fmt.Errorf("registry policy invalid: %s must be a JSON string array: %w", setting.Key, err)
	}
	if bytes.Equal(bytes.TrimSpace(setting.Value), []byte("null")) {
		return nil, fmt.Errorf("registry policy invalid: %s must be a JSON string array", setting.Key)
	}
	return normalizeOverwriteExclusions(exclusions)
}

func normalizeOverwriteExclusions(exclusions []string) ([]string, error) {
	if len(exclusions) > maxTagOverwriteExclusionEntries {
		return nil, fmt.Errorf("%w: overwrite exclusions must contain at most %d tags",
			ErrRegistryPolicyInvalid, maxTagOverwriteExclusionEntries)
	}
	normalized := make([]string, 0, len(exclusions))
	seen := make(map[string]struct{}, len(exclusions))
	for _, raw := range exclusions {
		tag := strings.TrimSpace(raw)
		if !registryTagPattern.MatchString(tag) {
			return nil, fmt.Errorf("%w: invalid overwrite exclusion tag %q", ErrRegistryPolicyInvalid, raw)
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func validateRegistryPolicy(p *RegistryPolicy) error {
	if p == nil {
		return fmt.Errorf("registry policy is nil")
	}
	if p.Version <= 0 {
		return fmt.Errorf("registry policy invalid: persisted version must be positive")
	}
	if p.UpdatedBy == "" {
		return fmt.Errorf("registry policy invalid: updated_by is empty")
	}
	normalized, err := normalizeOverwriteExclusions(p.OverwriteExclusions)
	if err != nil {
		return err
	}
	if !slices.Equal(normalized, p.OverwriteExclusions) {
		return fmt.Errorf("registry policy invalid: overwrite exclusions are not normalized")
	}
	return nil
}

func cloneRegistryPolicy(p *RegistryPolicy) *RegistryPolicy {
	if p == nil {
		return nil
	}
	clone := *p
	clone.OverwriteExclusions = slices.Clone(p.OverwriteExclusions)
	return &clone
}
