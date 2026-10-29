package k3s

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionFromChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable":
			http.Redirect(w, r, "https://github.com/k3s-io/k3s/releases/tag/v1.36.5+k3s1", http.StatusFound)
		case "/bad":
			http.Redirect(w, r, "https://github.com/k3s-io/k3s/releases", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	v, err := versionFromChannel(t.Context(), client, srv.URL+"/stable")
	require.NoError(t, err)
	assert.Equal(t, "v1.36.5+k3s1", v)

	_, err = versionFromChannel(t.Context(), client, srv.URL+"/bad")
	require.Error(t, err, "адрес без версии")

	_, err = versionFromChannel(t.Context(), client, srv.URL+"/nope")
	require.Error(t, err, "нет перенаправления")
}
