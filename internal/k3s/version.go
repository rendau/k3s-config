// Package k3s — сведения о выпусках k3s с update.k3s.io.
package k3s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rendau/k3s-config/internal/httpx"
)

// ChannelsURL — каналы выпусков: stable, v1.35 и т.п. Канал отвечает
// перенаправлением на страницу релиза, последний элемент адреса которой —
// версия, напр. v1.36.5+k3s1.
const ChannelsURL = "https://update.k3s.io/v1-release/channels/"

// StableVersion возвращает текущую стабильную версию k3s.
func StableVersion(ctx context.Context) (string, error) {
	return ChannelVersion(ctx, "stable")
}

// ChannelVersion возвращает последнюю версию канала.
func ChannelVersion(ctx context.Context, channel string) (string, error) {
	client := httpx.New(httpx.Config{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})

	return versionFromChannel(ctx, client, ChannelsURL+channel)
}

func versionFromChannel(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 3 {
		return "", fmt.Errorf("%s: ожидал перенаправление, получил %s", url, resp.Status)
	}
	location := resp.Header.Get("Location")
	version := path.Base(location)
	if !strings.HasPrefix(version, "v") || !strings.Contains(version, "+k3s") {
		return "", errors.New("не разобрал версию из адреса " + location)
	}

	return version, nil
}
