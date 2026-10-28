// Package ui печатает сообщения для человека в stderr.
package ui

import (
	"fmt"
	"os"
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
