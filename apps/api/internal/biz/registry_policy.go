package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"api/internal/data/ent"
	"api/internal/data/ent/systemsetting"

	"golang.org/x/sync/semaphore"
)

var (
	ErrRegistryPolicyNotInitialized = errors.New("registry policy not initialized")
	ErrRegistryPolicyConflict       = errors.New("registry policy version conflict")
)

// RegistryPolicy is the strongly typed registry-wide runtime policy decoded
// from its entry in the shared system_settings table.
type RegistryPolicy struct {
	PreventTagOverwrite bool
	Version             int64
	UpdatedAt           time.Time
	UpdatedBy           string
}

const SettingKeyPreventTagOverwrite = "registry.prevent_tag_overwrite"

const registryPolicyGateCapacity int64 = 1 << 30

// RegistryPolicyUsecase owns the in-memory policy snapshot and the switch
// barrier. Manifest PUTs hold one gate permit for their full lifetime;
// updates acquire the entire gate, so a successful update cleanly separates
// requests using the old policy from requests using the new one.
type RegistryPolicyUsecase struct {
	db       *ent.Client
	gate     *semaphore.Weighted
	snapshot atomic.Pointer[RegistryPolicy]
}

func NewRegistryPolicyUsecase(db *ent.Client) *RegistryPolicyUsecase {
	return &RegistryPolicyUsecase{
		db:   db,
		gate: semaphore.NewWeighted(registryPolicyGateCapacity),
	}
}

// Initialize loads the persisted value before the HTTP server starts. A
// missing key is represented as a virtual version-0 default and is not
// written: system_settings only records administrator changes.
func (u *RegistryPolicyUsecase) Initialize(ctx context.Context) error {
	setting, err := u.querySetting(ctx)
	if ent.IsNotFound(err) {
		u.snapshot.Store(&RegistryPolicy{
			PreventTagOverwrite: false,
			Version:             0,
			UpdatedBy:           "system",
		})
		return nil
	}
	if err != nil {
		return fmt.Errorf("initialize registry policy: %w", err)
	}
	p, err := registryPolicyFromSetting(setting)
	if err != nil {
		return err
	}
	if err := validateRegistryPolicy(p); err != nil {
		return err
	}
	u.snapshot.Store(cloneRegistryPolicy(p))
	return nil
}

func (u *RegistryPolicyUsecase) Get() (*RegistryPolicy, error) {
	p := u.snapshot.Load()
	if p == nil {
		return nil, ErrRegistryPolicyNotInitialized
	}
	return cloneRegistryPolicy(p), nil
}

// EnterManifestPut returns a stable policy snapshot and a release function.
// Callers must defer release until the upstream manifest PUT has completed.
func (u *RegistryPolicyUsecase) EnterManifestPut(ctx context.Context) (*RegistryPolicy, func(), error) {
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

// Update changes the policy with optimistic concurrency control. Acquiring
// the entire gate makes the update a linearization point for manifest PUTs.
func (u *RegistryPolicyUsecase) Update(
	ctx context.Context,
	expectedVersion int64,
	preventTagOverwrite bool,
	actor string,
) (*RegistryPolicy, error) {
	if expectedVersion < 0 {
		return nil, fmt.Errorf("expected version must not be negative")
	}
	if actor == "" {
		return nil, fmt.Errorf("actor is required")
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
	if current.PreventTagOverwrite == preventTagOverwrite {
		return cloneRegistryPolicy(current), nil
	}

	value, err := json.Marshal(preventTagOverwrite)
	if err != nil {
		return nil, fmt.Errorf("marshal registry policy: %w", err)
	}
	setting, err := u.updateSetting(ctx, expectedVersion, value, actor)
	if err != nil {
		if errors.Is(err, ErrRegistryPolicyConflict) {
			if latestSetting, getErr := u.querySetting(ctx); getErr == nil {
				if latest, parseErr := registryPolicyFromSetting(latestSetting); parseErr == nil && validateRegistryPolicy(latest) == nil {
					u.snapshot.Store(cloneRegistryPolicy(latest))
					return cloneRegistryPolicy(latest), ErrRegistryPolicyConflict
				}
			}
		}
		return nil, err
	}
	updated, err := registryPolicyFromSetting(setting)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryPolicy(updated); err != nil {
		return nil, err
	}
	u.snapshot.Store(cloneRegistryPolicy(updated))
	return cloneRegistryPolicy(updated), nil
}

func (u *RegistryPolicyUsecase) querySetting(ctx context.Context) (*ent.SystemSetting, error) {
	return u.db.SystemSetting.Query().
		Where(systemsetting.KeyEQ(SettingKeyPreventTagOverwrite)).
		Only(ctx)
}

func (u *RegistryPolicyUsecase) updateSetting(
	ctx context.Context,
	expectedVersion int64,
	value json.RawMessage,
	actor string,
) (*ent.SystemSetting, error) {
	if expectedVersion == 0 {
		created, err := u.db.SystemSetting.Create().
			SetKey(SettingKeyPreventTagOverwrite).
			SetValue(append(json.RawMessage(nil), value...)).
			SetVersion(1).
			SetUpdatedBy(actor).
			Save(ctx)
		if ent.IsConstraintError(err) {
			return nil, ErrRegistryPolicyConflict
		}
		return created, err
	}

	updated, err := u.db.SystemSetting.Update().
		Where(
			systemsetting.KeyEQ(SettingKeyPreventTagOverwrite),
			systemsetting.VersionEQ(expectedVersion),
		).
		SetValue(append(json.RawMessage(nil), value...)).
		SetVersion(expectedVersion + 1).
		SetUpdatedBy(actor).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, ErrRegistryPolicyConflict
	}
	return u.querySetting(ctx)
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
		return nil, fmt.Errorf("registry policy invalid: decode %s: %w", setting.Key, err)
	}
	enabled, ok := decoded.(bool)
	if !ok {
		return nil, fmt.Errorf("registry policy invalid: %s must be a JSON boolean", setting.Key)
	}
	return &RegistryPolicy{
		PreventTagOverwrite: enabled,
		Version:             setting.Version,
		UpdatedAt:           setting.UpdatedAt,
		UpdatedBy:           setting.UpdatedBy,
	}, nil
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
	return nil
}

func cloneRegistryPolicy(p *RegistryPolicy) *RegistryPolicy {
	if p == nil {
		return nil
	}
	clone := *p
	return &clone
}
