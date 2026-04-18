package biz

import "github.com/google/wire"

// ProviderSet is the biz layer provider set.
// Populated in M2 as usecases are introduced (user, permission, token, keystore, maintenance).
var ProviderSet = wire.NewSet()
