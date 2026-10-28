package logger

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Level defines log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
)

var levelColors = map[Level]string{
	LevelDebug: colorMagenta,
	LevelInfo:  colorCyan,
	LevelWarn:  colorYellow,
	LevelError: colorRed,
}

var (
	currentLevel = LevelInfo
	useColors    = true
	outMu        sync.Mutex
	outWriter    io.Writer = os.Stdout
	defaultLog             = New("app")
)

func init() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColors = false
	}
	if strings.ToLower(os.Getenv("DEBUG")) == "true" || os.Getenv("DEBUG") == "1" {
		currentLevel = LevelDebug
	}
}

// SetDebug toggles debug level logging.
func SetDebug(enabled bool) {
	if enabled {
		currentLevel = LevelDebug
	} else {
		currentLevel = LevelInfo
	}
}

// Logger provides leveled, prefixed logging.
type Logger struct {
	prefix string
}

// New creates a new Logger with the specified module prefix.
func New(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

func (l *Logger) log(lvl Level, msg string) {
	if lvl < currentLevel {
		return
	}

	outMu.Lock()
	defer outMu.Unlock()

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	lvlName := levelNames[lvl]

	if useColors {
		color := levelColors[lvl]
		fmt.Fprintf(outWriter, "%s%s%s %s[%-5s]%s %s[%s]%s %s\n",
			colorGray, timestamp, colorReset,
			color, lvlName, colorReset,
			colorBlue, l.prefix, colorReset,
			msg,
		)
	} else {
		fmt.Fprintf(outWriter, "%s [%-5s] [%s] %s\n",
			timestamp,
			lvlName,
			l.prefix,
			msg,
		)
	}
}

func (l *Logger) Debug(args ...interface{}) {
	l.log(LevelDebug, fmt.Sprint(args...))
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, fmt.Sprintf(format, args...))
}

func (l *Logger) Info(args ...interface{}) {
	l.log(LevelInfo, fmt.Sprint(args...))
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, fmt.Sprintf(format, args...))
}

func (l *Logger) Warn(args ...interface{}) {
	l.log(LevelWarn, fmt.Sprint(args...))
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, fmt.Sprintf(format, args...))
}

func (l *Logger) Error(args ...interface{}) {
	l.log(LevelError, fmt.Sprint(args...))
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...))
}

func (l *Logger) Fatal(args ...interface{}) {
	l.log(LevelError, fmt.Sprint(args...))
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Package-level global logging convenience functions
func Debug(args ...interface{})                 { defaultLog.Debug(args...) }
func Debugf(format string, args ...interface{}) { defaultLog.Debugf(format, args...) }
func Info(args ...interface{})                  { defaultLog.Info(args...) }
func Infof(format string, args ...interface{})  { defaultLog.Infof(format, args...) }
func Warn(args ...interface{})                  { defaultLog.Warn(args...) }
func Warnf(format string, args ...interface{})  { defaultLog.Warnf(format, args...) }
func Error(args ...interface{})                 { defaultLog.Error(args...) }
func Errorf(format string, args ...interface{}) { defaultLog.Errorf(format, args...) }
func Fatal(args ...interface{})                 { defaultLog.Fatal(args...) }
func Fatalf(format string, args ...interface{}) { defaultLog.Fatalf(format, args...) }

// HTTPMiddleware creates an HTTP request logger that can be enabled or disabled.
// When enabled is false, it passes through requests completely silent.
func HTTPMiddleware(enabled bool) func(next http.Handler) http.Handler {
	httpLog := New("http")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap response writer to capture status and bytes written
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			// Clean, formatted request log
			msg := fmt.Sprintf("%s %s -> %d (%s, %dB) from %s",
				r.Method,
				r.URL.Path,
				status,
				duration.Round(time.Millisecond),
				ww.BytesWritten(),
				r.RemoteAddr,
			)

			if status >= 500 {
				httpLog.Error(msg)
			} else if status >= 400 {
				httpLog.Warn(msg)
			} else {
				httpLog.Info(msg)
			}
		})
	}
}
