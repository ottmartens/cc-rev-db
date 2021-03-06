package logger

import (
	"fmt"
	"time"
)

type color string

type LoggingLevel int

var remoteConnected = false
var maxLogLevel = debug

var Levels levelsStruct = levelsStruct{
	Err:     err,
	Warn:    warn,
	Info:    info,
	Verbose: verbose,
	Debug:   debug,
}

const (
	reset      color = "\033[0m"
	cyan       color = "\033[36m"
	blue       color = "\033[34m"
	brightBlue color = "\033[94m"
	yellow     color = "\033[33m"
	red        color = "\033[31m"
)

const (
	err     LoggingLevel = 1
	warn    LoggingLevel = 2
	info    LoggingLevel = 3
	verbose LoggingLevel = 4
	debug   LoggingLevel = 5
)

type levelsStruct struct {
	Err     LoggingLevel
	Warn    LoggingLevel
	Info    LoggingLevel
	Verbose LoggingLevel
	Debug   LoggingLevel
}

var colorMap map[LoggingLevel]color = map[LoggingLevel]color{
	err:     red,
	warn:    yellow,
	info:    cyan,
	verbose: brightBlue,
	debug:   blue,
}

func SetMaxLogLevel(level LoggingLevel) {
	maxLogLevel = level
}

func Error(str string, args ...interface{}) {
	log(err, str, args...)
}

func Warn(str string, args ...interface{}) {
	log(warn, str, args...)
}

func Info(str string, args ...interface{}) {
	log(info, str, args...)
}

func Verbose(str string, args ...interface{}) {
	log(verbose, str, args...)
}

func Debug(str string, args ...interface{}) {
	log(debug, str, args...)
}

func log(level LoggingLevel, str string, args ...interface{}) {
	message := fmt.Sprintf(str, args...)

	if remoteLoggerClient == nil {
		logRow(level, message, nil)
	} else {
		logRemotely(level, message)
	}

}

func logRow(level LoggingLevel, message string, id *int) {
	if level > maxLogLevel {
		return
	}

	if id == nil {
		fmt.Printf("%s%s %s %s\n", colorMap[level], timeString(), message, reset)
	} else {
		fmt.Printf("%s%s %s -%s %s%s\n", colorMap[level], timeString(), prettyPrintId(*id), colorMap[level], message, reset)
	}

}

func timeString() string {
	hour, min, sec := time.Now().Clock()

	return fmt.Sprintf("%02d:%02d:%02d", hour, min, sec)
}

func prettyPrintId(id int) string {
	if id > 10 {
		return fmt.Sprint(id)
	}

	bold := "\033[1m"
	bg := "\033[38;5;226m"

	return fmt.Sprintf("%s%s%d%s", bold, bg, id, reset)
}
