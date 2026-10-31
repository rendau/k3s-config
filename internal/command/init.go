package command

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/k3s"
	"github.com/rendau/k3s-config/internal/kubecfg"
	"github.com/rendau/k3s-config/internal/ui"
)

// Init заводит файл сервера со стандартными настройками k3sctl. Один порядок
// на все случаи: сервера ещё нет (адрес не введён), сервер пустой или k3s на
// нём уже стоит, в том числе поставленный руками. Во втором и третьем случае
// заходит на сервер и читает его, ничего не меняя; с настроенного сервера
// берётся только то, без чего кластер сломается (см. applyServer).
func Init(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("укажите имя сервера: k3sctl init <имя>")
	}
	if err := config.CheckInit(name); err != nil {
		return err
	}

	conf, err := config.Defaults(name)
	if err != nil {
		return err
	}

	var notes []string

	host := ui.Ask("адрес сервера, можно с портом (Enter = сервера ещё нет): ", conf.SSHHost)
	if host != "" {
		conf.SSHHost, conf.SSHPort = splitHostPort(host, conf.SSHPort)
		conf.BootstrapUser = ui.Ask(fmt.Sprintf("логин для входа (Enter = %s): ", conf.BootstrapUser), conf.BootstrapUser)
		if notes, err = readServer(ctx, conf); err != nil {
			return err
		}
	}

	if conf.K3sVersion == "" {
		if conf.K3sVersion, err = k3s.StableVersion(ctx); err != nil {
			ui.Warn("не узнал стабильную версию k3s (%v): K3S_VERSION оставляю пустым, install возьмёт последнюю", err)
		}
	}
	// В файл попадает только признак, сам пароль — никогда.
	if conf.BootstrapPassword != "" {
		conf.BootstrapPassword = "ask"
	}

	path, err := config.Write(name, conf, notes)
	if err != nil {
		return err
	}

	ui.Info("создан %s", path)
	if conf.SSHKey != "" {
		ui.Info("ssh-ключ: %s (и %s)", conf.SSHKey, conf.SSHPubkey)
	} else {
		ui.Warn("ssh-ключ в ~/.ssh не найден (id_rsa, id_ed25519, id_ecdsa): впишите SSH_KEY и SSH_PUBKEY")
	}
	if conf.K3sVersion != "" {
		ui.Info("версия k3s: %s", conf.K3sVersion)
	}
	if conf.SSHHost == "" {
		ui.Info("дальше: k3sctl cloud-init %s (или впишите SSH_HOST и k3sctl prepare), install, access", name)
	} else {
		ui.Info("проверьте файл, дальше: k3sctl prepare %s, install", name)
	}

	return nil
}

// splitHostPort разбирает адрес вида host или host:port.
func splitHostPort(s string, port int) (string, int) {
	host, p, err := net.SplitHostPort(s)
	if err != nil {
		return s, port
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return s, port
	}

	return host, n
}

// readServer заходит на сервер и дополняет настройки тем, что на нём нашлось.
// Возвращает заметки для файла: что с сервера намеренно не перенесено.
func readServer(ctx context.Context, conf *config.Config) ([]string, error) {
	if conf.BootstrapPassword == "" {
		conf.BootstrapPassword = "ask"
	}

	boot, _, err := bootstrap(ctx, conf)
	if err != nil {
		return nil, err
	}
	if boot.Password == "" && boot.SudoPassword == "" {
		conf.BootstrapPassword = ""
	}

	ui.Info("читаю сервер %s, ничего не меняю", boot)
	info, err := inspect(ctx, boot)
	if err != nil {
		return nil, err
	}
	if info.Role == "" {
		ui.Info("k3s на сервере нет, настройки стандартные")

		return nil, nil
	}

	notes := applyServer(conf, info)
	ui.Info("k3s %s, роль %s; всё остальное в файле — стандарт k3sctl", info.Version, info.Role)
	for _, n := range notes {
		ui.Warn("%s", n)
	}

	master := conf.SSHHost
	if conf.K3sRole == "agent" {
		if conf.K3sServerSSHHost == "" {
			ui.Warn("не нашёл адрес master: впишите K3S_SERVER_URL и K3S_SERVER_SSH_HOST")
		}
		master = conf.K3sServerSSHHost
	}
	if found := kubecfg.FindContext(conf.LocalKubeconfig, master); found != "" {
		conf.ContextName = found
		ui.Info("контекст kubectl этого кластера: %s", found)
	} else {
		conf.ContextName = ui.Ask(fmt.Sprintf("контекст kubectl этого кластера (Enter = %s): ", conf.ContextName), conf.ContextName)
	}

	return notes, nil
}

// applyServer переносит в настройки то, без чего настроенный сервер сломается
// или изменится незаметно для владельца: роль, версию, адрес master, флаги,
// от которых зависит кластер, и вход по паролю. Остальное остаётся стандартом
// k3sctl, в этом и смысл: install приведёт сервер к нему. Возвращает заметки
// о том, что не перенесено.
func applyServer(conf *config.Config, info *serverInfo) []string {
	conf.K3sRole = info.Role
	conf.K3sVersion = info.Version

	// Закрытый пароль не открываем, открытый случайно не закрываем.
	conf.PrepareDisablePassword = info.PasswordAuth == "no"

	args := splitNodeArgs(info.NodeArgs)
	conf.K3sExtraArgs = strings.Join(args.Extra, " ")
	conf.K3sTLSSan = strings.Join(args.TLSSan, ",")

	conf.K3sServerURL = lo.CoalesceOrEmpty(info.ServerURL, args.ServerURL)
	if conf.K3sRole == "agent" {
		if u, err := url.Parse(conf.K3sServerURL); err == nil {
			conf.K3sServerSSHHost = u.Hostname()
		}
	}
	if info.NodeName != "" {
		conf.K3sInstallEnv = "K3S_NODE_NAME=" + info.NodeName
	}

	var notes []string
	if info.PasswordAuth == "" {
		notes = append(notes, "не узнал, включён ли вход по паролю: PREPARE_DISABLE_PASSWORD=false, проверьте")
	}
	if len(info.NodeArgs) == 0 {
		notes = append(notes, "не прочитал флаги k3s с ноды: проверьте K3S_EXTRA_ARGS и K3S_TLS_SAN")
	}
	if len(args.Kubelet) > 0 {
		notes = append(notes, "ручные настройки kubelet, вместо них стандарт; если какая-то нужна, перенесите в K3S_EXTRA_ARGS: "+strings.Join(args.Kubelet, " "))
	}
	if len(args.Skipped) > 0 {
		notes = append(notes, "значение с пробелами: "+strings.Join(args.Skipped, " "))
	}
	if len(args.Secrets) > 0 {
		notes = append(notes, "секреты: "+strings.Join(lo.Uniq(args.Secrets), " "))
	}

	other := lo.Without(info.EnvKeys, "K3S_URL", "K3S_TOKEN", "K3S_AGENT_TOKEN", "K3S_NODE_NAME")
	if len(other) > 0 {
		notes = append(notes, "переменные службы, install их сотрёт: "+strings.Join(other, " "))
	}

	return notes
}
