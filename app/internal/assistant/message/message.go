package message

import (
	"fmt"
	"time"

	"github.com/fatih/color"
)

func prefix() string {
	return fmt.Sprintf("[%v] ", time.Now().Format("2006.01.02 15:04:05"))
}
func flush() string {
	return "\r\033[K"
}

func ThrowFatal(msg string) {
	color.New(color.FgBlack, color.BgRed).Println(prefix() + msg)
}

// Debug
func Debug(msg string) {
	color.New(color.FgMagenta).Print(prefix() + msg)
}
func FlushDebug(msg string) {
	color.New(color.FgMagenta).Print(flush() + prefix() + msg)
}
func DebugF(format string, v ...any) {
	color.New(color.FgMagenta).Printf(prefix()+format, v...)
}
func FlushDebugF(format string, v ...any) {
	color.New(color.FgMagenta).Printf(flush()+prefix()+format, v...)
}

// Danger
func Danger(msg string) {
	color.New(color.FgRed).Print(prefix() + msg)
}
func FlushDanger(msg string) {
	color.New(color.FgRed).Print(flush() + prefix() + msg)
}
func DangerF(format string, v ...any) {
	color.New(color.FgRed).Printf(prefix()+format, v...)
}
func FlushDangerF(format string, v ...any) {
	color.New(color.FgRed).Printf(flush()+prefix()+format, v...)
}

// Warning
func Warning(msg string) {
	color.New(color.FgYellow).Print(prefix() + msg)
}
func FlushWarning(msg string) {
	color.New(color.FgYellow).Print(flush() + prefix() + msg)
}
func WarningF(format string, v ...any) {
	color.New(color.FgYellow).Printf(prefix()+format, v...)
}
func FlushWarningF(format string, v ...any) {
	color.New(color.FgYellow).Printf(flush()+prefix()+format, v...)
}

// Success
func Success(msg string) {
	color.New(color.FgGreen).Print(prefix() + msg)
}
func FlushSuccess(msg string) {
	color.New(color.FgGreen).Print(flush() + prefix() + msg)
}
func SuccessF(format string, v ...any) {
	color.New(color.FgGreen).Printf(prefix()+format, v...)
}
func FlushSuccessF(format string, v ...any) {
	color.New(color.FgGreen).Printf(flush()+prefix()+format, v...)
}

// Info
func Info(msg string) {
	color.New(color.FgCyan).Print(prefix() + msg)
}
func FlushInfo(msg string) {
	color.New(color.FgCyan).Print(flush() + prefix() + msg)
}
func InfoF(format string, v ...any) {
	color.New(color.FgCyan).Printf(prefix()+format, v...)
}
func FlushInfoF(format string, v ...any) {
	color.New(color.FgCyan).Printf(flush()+prefix()+format, v...)
}
