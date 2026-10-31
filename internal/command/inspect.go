package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rendau/k3s-config/internal/remote"
	"github.com/rendau/k3s-config/internal/sshx"
)

// secretFlags — флаги k3s, значения которых в файл настроек не попадают.
var secretFlags = map[string]bool{
	"token": true, "t": true, "token-file": true,
	"agent-token": true, "agent-token-file": true,
	"datastore-endpoint": true, "vpn-auth": true, "vpn-auth-file": true,
	"etcd-s3-access-key": true, "etcd-s3-secret-key": true,
}

// serverInfo — что на сервере стоит, по выводу inspect_server.sh.
type serverInfo struct {
	Role         string // server, agent; пусто = k3s нет
	Version      string
	Node         string // имя ноды в кластере
	NodeName     string // K3S_NODE_NAME из окружения службы
	ServerURL    string // K3S_URL из окружения службы
	PasswordAuth string // yes, no; пусто = не узнали
	EnvKeys      []string
	NodeArgs     []string
	Active       bool
	Managed      bool // config.yaml записан k3sctl
	Etcd         bool
}

// inspect читает с сервера, что на нём стоит. Ничего не меняет.
func inspect(ctx context.Context, t sshx.Target) (*serverInfo, error) {
	script, err := remote.Script(remote.ScriptInspectServer, nil)
	if err != nil {
		return nil, err
	}

	out, err := t.RunScript(ctx, script, true)
	if err != nil {
		return nil, err
	}

	return parseInspect(out)
}

func parseInspect(out string) (*serverInfo, error) {
	info := &serverInfo{}

	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "ROLE":
			info.Role = value
		case "VERSION":
			info.Version = value
		case "NODE":
			info.Node = value
		case "NODE_NAME":
			info.NodeName = value
		case "SERVER_URL":
			info.ServerURL = value
		case "PASSWORD_AUTH":
			info.PasswordAuth = value
		case "ENV_KEYS":
			info.EnvKeys = strings.FieldsFunc(value, func(r rune) bool { return r == ',' })
		case "ACTIVE":
			info.Active = value == "true"
		case "MANAGED":
			info.Managed = value == "true"
		case "ETCD":
			info.Etcd = value == "true"
		case "NODE_ARGS":
			if value == "" {
				continue
			}
			if err := json.Unmarshal([]byte(value), &info.NodeArgs); err != nil {
				return nil, fmt.Errorf("флаги ноды: %w", err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("вывод проверки сервера: %w", err)
	}

	return info, nil
}

// nodeArgs — флаги ноды, разобранные по тому, что с ними делать.
type nodeArgs struct {
	Extra     []string // нужны кластеру, идут в K3S_EXTRA_ARGS
	TLSSan    []string // K3S_TLS_SAN
	ServerURL string   // --server
	Kubelet   []string // ручные настройки kubelet: вместо них стандарт k3sctl
	Skipped   []string // значение не перенести строкой
	Secrets   []string // имена флагов с секретами
}

// splitNodeArgs разбирает значение аннотации k3s.io/node-args: первым идёт
// роль, дальше флаги, значение флага — следующим элементом или через «=».
func splitNodeArgs(args []string) nodeArgs {
	var res nodeArgs

	for i := 1; i < len(args); i++ {
		flag, value, hasValue := strings.Cut(args[i], "=")
		if !hasValue && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			value, hasValue = args[i], true
		}

		name := strings.TrimLeft(flag, "-")
		switch {
		case secretFlags[name] || strings.Contains(value, "***"):
			res.Secrets = append(res.Secrets, flag)
		case name == "tls-san":
			res.TLSSan = append(res.TLSSan, value)
		case name == "server" || name == "s":
			res.ServerURL = value
		case name == "kubelet-arg":
			res.Kubelet = append(res.Kubelet, flag+" "+value)
		case name == "write-kubeconfig-mode" || name == "config" || name == "c":
			// write-kubeconfig-mode задаёт сам install, config — путь к файлу, который он пишет
		case strings.ContainsAny(value, " \t"):
			res.Skipped = append(res.Skipped, flag+" "+value)
		case hasValue:
			res.Extra = append(res.Extra, flag, value)
		default:
			res.Extra = append(res.Extra, flag)
		}
	}

	return res
}
