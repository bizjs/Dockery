package biz

import "github.com/google/wire"

// ProviderSet wires all biz-layer constructors so cmd/api/wire.go can
// grab them with a single reference.
var ProviderSet = wire.NewSet(
	NewKeystoreConfigFromConf,
	NewKeystore,
	NewTokenIssuerConfigFromConf,
	NewTokenIssuer,
	NewUserUsecase,
	NewPermissionUsecase,
)
