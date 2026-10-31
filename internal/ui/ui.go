// Package ui печатает сообщения для человека в stderr.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Info — обычное сообщение о ходе работы.
func Info(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "==> "+format+"\n", a...)
}

// Warn — предупреждение, работа продолжается.
func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ВНИМАНИЕ: "+format+"\n", a...)
}

// Error — сообщение об ошибке.
func Error(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ОШИБКА: "+format+"\n", a...)
}

var stdin = bufio.NewReader(os.Stdin)

// Ask задаёт вопрос в терминале и возвращает ответ; пустой ответ или конец
// ввода — значение def.
func Ask(prompt, def string) string {
	fmt.Fprint(os.Stderr, prompt)

	line, _ := stdin.ReadString('\n')
	if line = strings.TrimSpace(line); line != "" {
		return line
	}

	return def
}

// Confirm спрашивает да/нет; всё, кроме y, — нет.
func Confirm(prompt string) bool {
	return strings.EqualFold(Ask(prompt+" [y/N] ", "n"), "y")
}
