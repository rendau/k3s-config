package k3s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstalledVersion(t *testing.T) {
	v, err := InstalledVersion("k3s version v1.33.5+k3s1 (ac39f19e)\ngo version go1.24.6")
	require.NoError(t, err)
	assert.Equal(t, "v1.33.5+k3s1", v)

	_, err = InstalledVersion("sh: k3s: not found")
	require.Error(t, err)

	_, err = InstalledVersion("k3s version dev (abc)")
	require.Error(t, err)
}

func TestCheckUpgrade(t *testing.T) {
	for _, tt := range []struct {
		name, from, to string
		ok             bool
	}{
		{"новый патч", "v1.33.5+k3s1", "v1.33.6+k3s1", true},
		{"новая сборка k3s", "v1.33.5+k3s1", "v1.33.5+k3s2", true},
		{"следующая минорная", "v1.33.5+k3s1", "v1.34.1+k3s1", true},
		{"кандидат в следующую минорную", "v1.33.5+k3s1", "v1.34.0-rc1+k3s1", true},
		{"через минорную", "v1.33.5+k3s1", "v1.35.0+k3s1", false},
		{"откат патча", "v1.33.5+k3s1", "v1.33.4+k3s1", false},
		{"откат сборки k3s", "v1.33.5+k3s2", "v1.33.5+k3s1", false},
		{"откат минорной", "v1.33.5+k3s1", "v1.32.9+k3s1", false},
		{"новая мажорная", "v1.33.5+k3s1", "v2.0.0+k3s1", false},
		{"мусор в целевой версии", "v1.33.5+k3s1", "latest", false},
		{"мусор в текущей версии", "dev", "v1.33.5+k3s1", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckUpgrade(tt.from, tt.to)
			if tt.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCheckUpgrade_MinorSkip(t *testing.T) {
	require.ErrorIs(t, CheckUpgrade("v1.33.5+k3s1", "v1.35.0+k3s1"), ErrMinorSkip)
	require.NotErrorIs(t, CheckUpgrade("v1.33.5+k3s1", "v1.32.9+k3s1"), ErrMinorSkip, "откат — другая ошибка")
}

func TestNextMinor(t *testing.T) {
	channel, err := NextMinor("v1.34.12+k3s1")
	require.NoError(t, err)
	assert.Equal(t, "v1.35", channel)

	_, err = NextMinor("dev")
	require.Error(t, err)
}

func TestCheckAgainstMaster(t *testing.T) {
	require.NoError(t, CheckAgainstMaster("v1.34.1+k3s1", "v1.34.1+k3s1"))
	require.NoError(t, CheckAgainstMaster("v1.34.1+k3s1", "v1.33.5+k3s1"))
	require.Error(t, CheckAgainstMaster("v1.33.5+k3s1", "v1.34.1+k3s1"))
	require.Error(t, CheckAgainstMaster("dev", "v1.34.1+k3s1"))
}
