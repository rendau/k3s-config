package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/k3s_config_merge/internal/config"
)

func TestLoadPubkeys(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "id_rsa.pub")
	colleague := filepath.Join(dir, "colleague.pub")
	require.NoError(t, os.WriteFile(main, []byte("ssh-rsa AAAA main\n"), 0o600))
	require.NoError(t, os.WriteFile(colleague, []byte("ssh-ed25519 BBBB colleague\n"), 0o600))

	keys, err := loadPubkeys(&config.Config{SSHPubkey: main})
	require.NoError(t, err)
	assert.Equal(t, []string{"ssh-rsa AAAA main"}, keys, "только основной")

	keys, err = loadPubkeys(&config.Config{
		SSHPubkey:       main,
		SSHExtraPubkeys: colleague + ", ssh-ed25519 CCCC literal ,, " + main,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ssh-rsa AAAA main", "ssh-ed25519 BBBB colleague", "ssh-ed25519 CCCC literal"}, keys,
		"файл, ключ строкой, повтор основного убран")

	_, err = loadPubkeys(&config.Config{SSHPubkey: main, SSHExtraPubkeys: filepath.Join(dir, "nope.pub")})
	require.Error(t, err, "несуществующий файл")

	_, err = loadPubkeys(&config.Config{})
	require.Error(t, err, "без основного ключа")
}
