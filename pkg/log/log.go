//go:build !release

package log

import stdlog "log"

func Printf(format string, v ...interface{}) {
	stdlog.Printf(format, v...)
}
func Println(v ...interface{}) {
	stdlog.Println(v...)
}
func Fatal(v ...interface{}) {
	stdlog.Fatal(v...)
}
