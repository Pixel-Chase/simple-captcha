package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"simple-captcha/config"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// Logger 结构化日志器
type Logger struct {
	level  LogLevel
	format string
	output io.Writer
	mu     sync.Mutex
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// LogEntry 日志条目
type LogEntry struct {
	Time    string                 `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
	File    string                 `json:"file,omitempty"`
	Line    int                    `json:"line,omitempty"`
}

// GetLogger 获取默认日志器
func GetLogger() *Logger {
	once.Do(func() {
		defaultLogger = NewLogger()
	})
	return defaultLogger
}

// NewLogger 创建新的日志器
func NewLogger() *Logger {
	cfg := config.GetConfig()

	level := parseLogLevel(cfg.Logging.Level)
	format := cfg.Logging.Format

	var output io.Writer
	switch cfg.Logging.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		// 文件输出
		if err := os.MkdirAll(filepath.Dir(cfg.Logging.Output), 0755); err != nil {
			log.Printf("Failed to create log directory: %v", err)
			output = os.Stdout
		} else {
			file, err := os.OpenFile(cfg.Logging.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				log.Printf("Failed to open log file: %v", err)
				output = os.Stdout
			} else {
				output = file
			}
		}
	}

	return &Logger{
		level:  level,
		format: format,
		output: output,
	}
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Debug 调试日志
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	l.log(DEBUG, message, fields...)
}

// Info 信息日志
func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	l.log(INFO, message, fields...)
}

// Warn 警告日志
func (l *Logger) Warn(message string, fields ...map[string]interface{}) {
	l.log(WARN, message, fields...)
}

// Error 错误日志
func (l *Logger) Error(message string, fields ...map[string]interface{}) {
	l.log(ERROR, message, fields...)
}

// Fatal 致命错误日志
func (l *Logger) Fatal(message string, fields ...map[string]interface{}) {
	l.log(FATAL, message, fields...)
	os.Exit(1)
}

// Debugf 格式化调试日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DEBUG, fmt.Sprintf(format, args...))
}

// Infof 格式化信息日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(INFO, fmt.Sprintf(format, args...))
}

// Warnf 格式化警告日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WARN, fmt.Sprintf(format, args...))
}

// Errorf 格式化错误日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ERROR, fmt.Sprintf(format, args...))
}

// Fatalf 格式化致命错误日志
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FATAL, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// WithFields 带字段的日志
func (l *Logger) WithFields(fields map[string]interface{}) *LoggerWithFields {
	return &LoggerWithFields{
		logger: l,
		fields: fields,
	}
}

// log 内部日志方法
func (l *Logger) log(level LogLevel, message string, fields ...map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Time:    time.Now().Format(time.RFC3339),
		Level:   levelNames[level],
		Message: message,
	}

	// 合并字段
	if len(fields) > 0 {
		entry.Fields = make(map[string]interface{})
		for _, fieldMap := range fields {
			for k, v := range fieldMap {
				entry.Fields[k] = v
			}
		}
	}

	// 获取调用者信息
	if _, file, line, ok := runtime.Caller(2); ok {
		entry.File = filepath.Base(file)
		entry.Line = line
	}

	var output string
	if l.format == "json" {
		jsonData, _ := json.Marshal(entry)
		output = string(jsonData) + "\n"
	} else {
		// 文本格式
		output = l.formatText(entry)
	}

	l.output.Write([]byte(output))
}

// formatText 格式化文本输出
func (l *Logger) formatText(entry LogEntry) string {
	var builder strings.Builder

	// 时间和级别
	builder.WriteString(fmt.Sprintf("[%s] %s ", entry.Time, entry.Level))

	// 文件和行号
	if entry.File != "" {
		builder.WriteString(fmt.Sprintf("%s:%d ", entry.File, entry.Line))
	}

	// 消息
	builder.WriteString(entry.Message)

	// 字段
	if entry.Fields != nil && len(entry.Fields) > 0 {
		builder.WriteString(" ")
		for k, v := range entry.Fields {
			builder.WriteString(fmt.Sprintf("%s=%v ", k, v))
		}
	}

	builder.WriteString("\n")
	return builder.String()
}

// LoggerWithFields 带字段的日志器
type LoggerWithFields struct {
	logger *Logger
	fields map[string]interface{}
}

// Debug 调试日志
func (l *LoggerWithFields) Debug(message string) {
	l.logger.log(DEBUG, message, l.fields)
}

// Info 信息日志
func (l *LoggerWithFields) Info(message string) {
	l.logger.log(INFO, message, l.fields)
}

// Warn 警告日志
func (l *LoggerWithFields) Warn(message string) {
	l.logger.log(WARN, message, l.fields)
}

// Error 错误日志
func (l *LoggerWithFields) Error(message string) {
	l.logger.log(ERROR, message, l.fields)
}

// Fatal 致命错误日志
func (l *LoggerWithFields) Fatal(message string) {
	l.logger.log(FATAL, message, l.fields)
	os.Exit(1)
}

// parseLogLevel 解析日志级别
func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// 全局日志函数
func Debug(message string, fields ...map[string]interface{}) {
	GetLogger().Debug(message, fields...)
}

func Info(message string, fields ...map[string]interface{}) {
	GetLogger().Info(message, fields...)
}

func Warn(message string, fields ...map[string]interface{}) {
	GetLogger().Warn(message, fields...)
}

func Error(message string, fields ...map[string]interface{}) {
	GetLogger().Error(message, fields...)
}

func Fatal(message string, fields ...map[string]interface{}) {
	GetLogger().Fatal(message, fields...)
}

func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}

func WithFields(fields map[string]interface{}) *LoggerWithFields {
	return GetLogger().WithFields(fields)
}
