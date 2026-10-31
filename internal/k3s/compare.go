package k3s

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ErrMinorSkip — переход через минорную версию.
var ErrMinorSkip = errors.New("минорные версии обновляются по одной")

var versionRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-[^+]*)?\+k3s(\d+)$`)

// version — разобранная версия k3s: major, minor, patch и номер сборки k3s.
type version [4]int

func parseVersion(s string) (version, error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return version{}, fmt.Errorf("не разобрал версию k3s %q, ожидаю вид v1.33.5+k3s1", s)
	}

	var v version
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return version{}, fmt.Errorf("не разобрал версию k3s %q: %w", s, err)
		}
		v[i] = n
	}

	return v, nil
}

// InstalledVersion достаёт версию из вывода k3s --version:
// «k3s version v1.33.5+k3s1 (ac39f19e)».
func InstalledVersion(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) < 3 || fields[0] != "k3s" || fields[1] != "version" {
		return "", fmt.Errorf("не разобрал вывод k3s --version: %q", output)
	}
	if _, err := parseVersion(fields[2]); err != nil {
		return "", err
	}

	return fields[2], nil
}

// CheckUpgrade проверяет, можно ли перейти с версии from на to: k3s не
// поддерживает откат назад и обновление через минорную версию.
func CheckUpgrade(from, to string) error {
	f, err := parseVersion(from)
	if err != nil {
		return err
	}
	t, err := parseVersion(to)
	if err != nil {
		return err
	}

	if slices.Compare(t[:], f[:]) < 0 {
		return fmt.Errorf("откат с %s на %s не поддерживается", from, to)
	}
	if t[0] != f[0] || t[1]-f[1] > 1 {
		return fmt.Errorf("с %s на %s нельзя за один шаг: %w", from, to, ErrMinorSkip)
	}

	return nil
}

// NextMinor — канал следующей минорной версии: v1.34.12+k3s1 → v1.35.
func NextMinor(s string) (string, error) {
	v, err := parseVersion(s)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("v%d.%d", v[0], v[1]+1), nil
}

// CheckAgainstMaster проверяет, что worker не обгонит master: в кластере
// сначала обновляется master, потом workers.
func CheckAgainstMaster(master, agent string) error {
	m, err := parseVersion(master)
	if err != nil {
		return err
	}
	a, err := parseVersion(agent)
	if err != nil {
		return err
	}

	if slices.Compare(a[:], m[:]) > 0 {
		return fmt.Errorf("на master %s, worker нельзя обновлять до %s: сначала обновите master", master, agent)
	}

	return nil
}
