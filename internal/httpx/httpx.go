// Package httpx — единая фабрика http-клиентов: таймауты соединения и TLS,
// лимит соединений на хост заданы явно, а не дефолтами net/http.
package httpx

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Config — настройки клиента; нулевые поля заменяются умолчаниями.
type Config struct {
	DialTimeout     time.Duration // установка соединения, по умолчанию 2s
	TLSTimeout      time.Duration // TLS-handshake, по умолчанию 2s
	MaxConnsPerHost int           // по умолчанию 20
	Timeout         time.Duration // общий таймаут запроса; 0 = без лимита (ограничивать контекстом)
	// CheckRedirect — политика перенаправлений, nil = следовать (как в net/http).
	CheckRedirect func(req *http.Request, via []*http.Request) error
}

// New создаёт клиент с настроенным транспортом.
func New(c Config) *http.Client {
	if c.DialTimeout == 0 {
		c.DialTimeout = 2 * time.Second
	}
	if c.TLSTimeout == 0 {
		c.TLSTimeout = 2 * time.Second
	}
	if c.MaxConnsPerHost == 0 {
		c.MaxConnsPerHost = 20
	}

	return &http.Client{
		Timeout:       c.Timeout,
		CheckRedirect: c.CheckRedirect,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: c.DialTimeout}).DialContext,
			TLSHandshakeTimeout: c.TLSTimeout,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // внутренние сервисы с самоподписанными сертификатами
			MaxConnsPerHost:     c.MaxConnsPerHost,
			MaxIdleConnsPerHost: c.MaxConnsPerHost,
		},
	}
}
