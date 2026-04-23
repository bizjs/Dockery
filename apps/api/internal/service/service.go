package service

import "github.com/google/wire"

// ProviderSet wires every kratoscarf-router-based service into a single
// Services bundle so the HTTP server receives one dependency instead of
// a positional list that grows with every new endpoint group.
var ProviderSet = wire.NewSet(
	NewSystemService,
	NewAuthService,
	NewUserService,
	NewPermissionService,
	NewRegistryService,
	NewTokenService,
	NewAdminService,
	NewWebhookService,
	wire.Struct(new(Services), "*"),
)

// Services aggregates all Dockery service objects.
type Services struct {
	System     *SystemService
	Auth       *AuthService
	User       *UserService
	Permission *PermissionService
	Registry   *RegistryService
	Token      *TokenService
	Admin      *AdminService
	Webhook    *WebhookService
}
