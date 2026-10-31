package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/k3s"
	"github.com/rendau/k3s-config/internal/kubecfg"
	"github.com/rendau/k3s-config/internal/remote"
	"github.com/rendau/k3s-config/internal/sshx"
	"github.com/rendau/k3s-config/internal/ui"
)

const (
	nodeReadyWait  = 5 * time.Minute
	nodeReadyRetry = 5 * time.Second
)

// Upgrade обновляет k3s на ноде. Если в K3S_VERSION стоит версия новее
// установленной, обновляет до неё; если пусто или там текущая версия, сам
// находит следующую (см. autoVersion) и после обновления записывает её в файл
// path: иначе в файле останется старая версия, и install откатит k3s назад.
// Меняет только бинарник (upgrade_k3s.sh): службу и config.yaml не трогает,
// роль берёт с самой ноды, поэтому работает на любой ноде, как бы её ни
// поставили. До замены проверяет, что переход допустим и кластер здоров,
// после — что нода вернулась на новой версии.
func Upgrade(ctx context.Context, c *config.Config, path string) error {
	if c.SSHHost == "" {
		return config.Missing("SSH_HOST")
	}
	if c.SSHUser == "" {
		return config.Missing("SSH_USER")
	}

	node := target(c, c.SSHUser)
	info, err := inspect(ctx, node)
	if err != nil {
		return fmt.Errorf("%w (нужен sudo без пароля: k3sctl prepare)", err)
	}
	if info.Role == "" {
		return fmt.Errorf("на %s нет k3s: поставить — k3sctl install", c.SSHHost)
	}

	nodes, err := kubecfg.Nodes(ctx, c.LocalKubeconfig, c.ContextName)
	if err != nil {
		if info.Role == "agent" {
			return fmt.Errorf("без доступа к кластеру нельзя проверить, что master уже обновлён: %w", err)
		}
		ui.Warn("кластер не проверяю, нет доступа через контекст %s: %v", c.ContextName, err)
	}

	wanted := c.K3sVersion
	if wanted == "" || wanted == info.Version {
		if wanted = autoVersion(ctx, info, nodes); wanted == "" {
			ui.Info("на %s стоит %s, новее версии не нашёл", c.SSHHost, info.Version)

			return nil
		}
	}

	if err = k3s.CheckUpgrade(info.Version, wanted); err != nil {
		if errors.Is(err, k3s.ErrMinorSkip) {
			if next := nextVersion(ctx, info.Version); next != "" {
				return fmt.Errorf("%w; сначала поставьте %s", err, next)
			}
		}

		return err
	}

	if err = checkCluster(nodes, info.Role, wanted); err != nil {
		return err
	}

	service := "k3s"
	if info.Role == "agent" {
		service = "k3s-agent"
	}

	ui.Info("план: %s (%s) %s → %s", lo.CoalesceOrEmpty(info.Node, c.SSHHost), service, info.Version, wanted)
	if info.Etcd {
		ui.Info("перед заменой — снапшот etcd")
	}
	if !ui.Confirm("Обновить?") {
		ui.Info("отменено, ничего не менял")

		return nil
	}

	script, err := remote.Script(remote.ScriptUpgradeK3s, map[string]string{
		"K3S_VERSION": wanted,
		"K3S_SERVICE": service,
	})
	if err != nil {
		return err
	}
	if _, err = node.RunScript(ctx, script, true); err != nil {
		return err
	}

	current, err := installedVersion(ctx, node)
	if err != nil {
		return err
	}
	if current != wanted {
		return fmt.Errorf("на %s осталась версия %s вместо %s", c.SSHHost, current, wanted)
	}
	if err = config.SetK3sVersion(path, wanted); err != nil {
		return err
	}

	if lo.ContainsBy(nodes, func(n kubecfg.Node) bool { return n.Name == info.Node }) {
		ui.Info("жду ноду %s в Ready на %s (до %s; API master может не отвечать 1–2 минуты)", info.Node, wanted, nodeReadyWait)
		if err = waitNode(ctx, c, info.Node, wanted); err != nil {
			return err
		}
	}
	ui.Info("k3s обновлён: %s", current)

	return nil
}

// checkCluster не даёт обновлять больной кластер и пускать worker вперёд master.
func checkCluster(nodes []kubecfg.Node, role, wanted string) error {
	notReady := lo.FilterMap(nodes, func(n kubecfg.Node, _ int) (string, bool) { return n.Name, !n.Ready })
	if len(notReady) > 0 {
		return fmt.Errorf("есть ноды не в Ready: %v", notReady)
	}
	if role != "agent" {
		return nil
	}
	for _, n := range nodes {
		if !n.ControlPlane {
			continue
		}
		if err := k3s.CheckAgainstMaster(n.Version, wanted); err != nil {
			return err
		}
	}

	return nil
}

// autoVersion выбирает версию, когда она не задана явно. Worker догоняет
// master: так версии в кластере совпадают, даже если в канале уже вышла более
// новая. В остальных случаях — следующая версия (nextVersion).
func autoVersion(ctx context.Context, info *serverInfo, nodes []kubecfg.Node) string {
	if info.Role == "agent" {
		master, ok := lo.Find(nodes, func(n kubecfg.Node) bool {
			return n.ControlPlane && n.Version != info.Version && k3s.CheckUpgrade(info.Version, n.Version) == nil
		})
		if ok {
			return master.Version
		}
	}

	return nextVersion(ctx, info.Version)
}

// nextVersion — на что обновляться дальше с версии current: стабильная, а
// если до неё больше одной минорной — последняя из следующей минорной.
// Пусто — обновляться некуда или не удалось узнать.
func nextVersion(ctx context.Context, current string) string {
	stable, err := k3s.StableVersion(ctx)
	if err != nil {
		ui.Warn("не узнал стабильную версию k3s: %v", err)

		return ""
	}
	if stable == current {
		return ""
	}

	err = k3s.CheckUpgrade(current, stable)
	if err == nil {
		return stable
	}
	if !errors.Is(err, k3s.ErrMinorSkip) {
		return ""
	}

	channel, err := k3s.NextMinor(current)
	if err != nil {
		return ""
	}
	next, err := k3s.ChannelVersion(ctx, channel)
	if err != nil {
		ui.Warn("не узнал версию канала %s: %v", channel, err)

		return ""
	}

	return next
}

// waitNode ждёт ноду в Ready на нужной версии; пока API перезапускается, ошибки не считаются.
func waitNode(ctx context.Context, c *config.Config, name, version string) error {
	deadline := time.Now().Add(nodeReadyWait)

	for {
		nodes, err := kubecfg.Nodes(ctx, c.LocalKubeconfig, c.ContextName)
		if err == nil && lo.ContainsBy(nodes, func(n kubecfg.Node) bool {
			return n.Name == name && n.Ready && n.Version == version
		}) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("нода %s не дошла до Ready на %s за %s", name, version, nodeReadyWait)
		}
		if err = sleepCtx(ctx, nodeReadyRetry); err != nil {
			return err
		}
	}
}

// installedVersion — версия k3s, установленная на сервере.
func installedVersion(ctx context.Context, t sshx.Target) (string, error) {
	output, err := t.Output(ctx, "k3s --version")
	if err != nil {
		return "", fmt.Errorf("версия k3s: %w", err)
	}

	return k3s.InstalledVersion(output)
}
