// Package remote хранит серверные sh-скрипты и шаблон cloud-init, встроенные
// в бинарник, и собирает из них текст для отправки на сервер.
package remote

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/samber/lo"
)

//go:embed scripts/*.sh templates/*.tpl
var files embed.FS

// Имена серверных скриптов (POSIX sh, каталог scripts/).
const (
	ScriptPrepareServer = "prepare_server.sh"
	ScriptInstallK3s    = "install_k3s.sh"
	ScriptSetupSA       = "setup_sa.sh"
	ScriptInspectServer = "inspect_server.sh"
	ScriptUpgradeK3s    = "upgrade_k3s.sh"
)

// Script собирает серверный скрипт: сначала строки NAME='value' с настройками
// (в алфавитном порядке), затем тело скрипта из scripts/NAME.
func Script(name string, vars map[string]string) (string, error) {
	body, err := files.ReadFile("scripts/" + name)
	if err != nil {
		return "", fmt.Errorf("встроенный скрипт %s: %w", name, err)
	}

	keys := lo.Keys(vars)
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, ShellQuote(vars[k]))
	}
	b.Write(body)

	return b.String(), nil
}

// ShellQuote заключает значение в одинарные кавычки для sh.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// CloudInitParams — подстановки в шаблон cloud-init.
type CloudInitParams struct {
	SSHUser       string
	SSHPubkeys    []string
	Timezone      string
	RebootCron    string // расписание cron перезагрузки после обновлений; пусто = не заводить
	JournalMaxUse string // потолок journald (SystemMaxUse); пусто = не трогать

	DisablePassword bool // запретить вход по паролю
}

// CloudInit печатает user-data для cloud-init по шаблону templates/cloud-init.yaml.tpl.
func CloudInit(p CloudInitParams) (string, error) {
	tpl, err := template.ParseFS(files, "templates/cloud-init.yaml.tpl")
	if err != nil {
		return "", fmt.Errorf("шаблон cloud-init: %w", err)
	}

	var buf bytes.Buffer
	if err = tpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("шаблон cloud-init: %w", err)
	}

	return buf.String(), nil
}
