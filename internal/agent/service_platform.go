package agent

import (
	"context"
	"reia-clash/internal/platform/coreservice"
)

type servicePlatform interface {
	Supported() bool
	RegistrationStatus() coreservice.Registration
	RequestInstall(context.Context, string) error
	RequestUninstall(context.Context, string) error
}
type systemServicePlatform struct{}

func (systemServicePlatform) Supported() bool { return coreservice.Supported() }
func (systemServicePlatform) RegistrationStatus() coreservice.Registration {
	return coreservice.RegistrationStatus()
}
func (systemServicePlatform) RequestInstall(ctx context.Context, path string) error {
	return coreservice.RequestInstall(ctx, path)
}
func (systemServicePlatform) RequestUninstall(ctx context.Context, root string) error {
	return coreservice.RequestUninstall(ctx, root)
}
func (a *App) serviceProvider() servicePlatform {
	if a.services != nil {
		return a.services
	}
	return systemServicePlatform{}
}
