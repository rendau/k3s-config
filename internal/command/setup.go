package command

import (
	"context"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/ui"
)

// Setup — всё разом для нового сервера: Prepare, Install и, если роль
// server, Access. Каждый шаг сам по себе повторяем, поэтому при ошибке
// достаточно исправить причину и запустить setup снова.
func Setup(ctx context.Context, c *config.Config) error {
	ui.Info("шаг 1/3: prepare")
	if err := Prepare(ctx, c); err != nil {
		return err
	}

	ui.Info("шаг 2/3: install")
	if err := Install(ctx, c); err != nil {
		return err
	}

	if c.K3sRole != "server" {
		ui.Info("шаг 3/3: access пропущен — роль %s, доступ к кластеру через master", c.K3sRole)

		return nil
	}
	ui.Info("шаг 3/3: access")

	return Access(ctx, c)
}
