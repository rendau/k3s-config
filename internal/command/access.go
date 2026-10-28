package command

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rendau/k3s_config_merge/internal/config"
	"github.com/rendau/k3s_config_merge/internal/kubecfg"
	"github.com/rendau/k3s_config_merge/internal/remote"
	"github.com/rendau/k3s_config_merge/internal/ui"
)

// Access заводит в кластере ServiceAccount с ролью, получает токен и
// корневой сертификат и записывает контекст в LOCAL_KUBECONFIG.
// Админский k3s.yaml на локальную машину не попадает.
func Access(ctx context.Context, c *config.Config) error {
	for _, v := range []struct{ name, value string }{
		{"SSH_HOST", c.SSHHost},
		{"SSH_USER", c.SSHUser},
		{"SA_NAME", c.SAName},
		{"CONTEXT_NAME", c.ContextName},
	} {
		if v.value == "" {
			return config.Missing(v.name)
		}
	}

	apiURL := c.APIURL
	if apiURL == "" {
		apiURL = fmt.Sprintf("https://%s:6443", c.SSHHost)
	}

	script, err := remote.Script(remote.ScriptSetupSA, map[string]string{
		"SA_NAME":      c.SAName,
		"SA_NAMESPACE": c.SANamespace,
		"CLUSTER_ROLE": c.ClusterRole,
		"SA_TOKEN_TTL": c.SATokenTTL,
	})
	if err != nil {
		return err
	}

	ui.Info("настраиваю учётку %s/%s на %s", c.SANamespace, c.SAName, c.SSHHost)
	out, err := target(c, c.SSHUser).RunScript(ctx, script, false)
	if err != nil {
		return err
	}

	ca, token, err := parseAccessOutput(out)
	if err != nil {
		return err
	}

	ui.Info("записываю контекст %s в %s", c.ContextName, c.LocalKubeconfig)
	err = kubecfg.Upsert(c.LocalKubeconfig, kubecfg.Entry{
		Context: c.ContextName,
		Cluster: c.ContextName,
		User:    c.SAName + "@" + c.ContextName,
		Server:  apiURL,
		CA:      ca,
		Token:   token,
	}, c.SetCurrentContext)
	if err != nil {
		return err
	}

	ui.Info("проверяю доступ")
	access, err := kubecfg.Verify(ctx, c.LocalKubeconfig, c.ContextName)
	if err != nil {
		return err
	}

	ui.Info("вошли как %s, версия API %s", access.Username, access.ServerVersion)
	ui.Info("готово: kubectl --context %s get pods -A", c.ContextName)

	return nil
}

// parseAccessOutput разбирает stdout серверного скрипта: строки CA=<base64 PEM>
// и TOKEN=<токен>. Возвращает сертификат в PEM и токен.
func parseAccessOutput(out string) ([]byte, string, error) {
	caB64, token := "", ""

	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "CA="):
			caB64 = strings.TrimPrefix(line, "CA=")
		case strings.HasPrefix(line, "TOKEN="):
			token = strings.TrimPrefix(line, "TOKEN=")
		}
	}
	if caB64 == "" || token == "" {
		return nil, "", fmt.Errorf("сервер не вернул сертификат и токен")
	}

	ca, err := base64.StdEncoding.DecodeString(caB64)
	if err != nil {
		return nil, "", fmt.Errorf("сертификат пришёл не в base64: %w", err)
	}
	if !bytes.Contains(ca, []byte("BEGIN CERTIFICATE")) {
		return nil, "", fmt.Errorf("сертификат пришёл в неожиданном виде")
	}

	return ca, token, nil
}
