package command

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAccessOutput(t *testing.T) {
	pem := "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n"
	out := "serviceaccount/x created\nCA=" + base64.StdEncoding.EncodeToString([]byte(pem)) + "\nTOKEN=eyJ.abc\n"

	ca, token, err := parseAccessOutput(out)
	require.NoError(t, err)
	assert.Equal(t, pem, string(ca))
	assert.Equal(t, "eyJ.abc", token)

	_, _, err = parseAccessOutput("TOKEN=abc\n")
	require.Error(t, err, "без сертификата")

	_, _, err = parseAccessOutput("CA=bm90IGEgY2VydA==\nTOKEN=abc\n")
	require.Error(t, err, "в base64 не сертификат")
}

func TestIsPubkey(t *testing.T) {
	assert.True(t, isPubkey("ssh-ed25519 AAAA user@host"))
	assert.True(t, isPubkey("ecdsa-sha2-nistp256 AAAA"))
	assert.True(t, isPubkey("sk-ssh-ed25519@openssh.com AAAA"))
	assert.False(t, isPubkey("-----BEGIN OPENSSH PRIVATE KEY-----"))
	assert.False(t, isPubkey(""))
}
