package biz

import (
	"github.com/bizjs/kratoscarf/auth/session"
	"github.com/google/wire"
)

// ProviderSet wires all biz-layer constructors so cmd/api/wire.go can
// grab them with a single reference.
//
// Session management is delegated to kratoscarf's auth/session package:
// NewMemoryStore + NewManager + wire.Bind wires up a ready *Manager
// that middleware and handlers consume via session.FromContext.
var ProviderSet = wire.NewSet(
	NewKeystoreConfigFromConf,
	NewKeystore,
	NewTokenIssuerConfigFromConf,
	NewTokenIssuer,
	NewSessionConfigFromConf,
	NewSessionManager,
	session.NewMemoryStore,
	wire.Bind(new(session.Store), new(*session.MemoryStore)),
	NewUserUsecase,
	NewPermissionUsecase,
	NewAuditUsecase,
	NewMaintenance,
	NewGCConfigFromConf,
	NewGCRunner,
	NewWebhookSecretConfigFromConf,
	NewWebhookSecret,
	NewRegistryUpstreamURL,
	NewRepoMetaUsecase,
	NewReconcilerConfigFromConf,
	NewReconciler,
)
