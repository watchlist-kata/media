package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"log/slog"
)

// ANSI color codes for log levels
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

// KafkaHandler sends logs to Kafka topic asynchronously.
type KafkaHandler struct {
	producer  sarama.AsyncProducer
	topic     string
	logChan   chan slog.Record
	wg        sync.WaitGroup
	quitChan  chan struct{}
	saramaCfg *sarama.Config
}

// NewKafkaHandler initializes a new KafkaHandler.
func NewKafkaHandler(brokers []string, topic string, bufferSize int) (*KafkaHandler, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = true
	config.Producer.Partitioner = sarama.NewHashPartitioner

	producer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create async producer: %w", err)
	}

	handler := &KafkaHandler{
		producer:  producer,
		topic:     topic,
		logChan:   make(chan slog.Record, bufferSize),
		quitChan:  make(chan struct{}),
		saramaCfg: config,
	}

	handler.wg.Add(1)
	go handler.processLogs()

	handler.wg.Add(1)
	go handler.handleProducerErrors()

	return handler, nil
}

// processLogs sends logs into channel for asynchronous processing.
func (k *KafkaHandler) processLogs() {
	defer k.wg.Done()
	for {
		select {
		case record := <-k.logChan:
			logEntry := map[string]interface{}{
				"time":  record.Time.Format(time.RFC3339),
				"level": record.Level.String(),
				"msg":   record.Message,
			}
			payload, err := json.Marshal(logEntry)
			if err != nil {
				fmt.Printf("failed to marshal log entry: %v\n", err)
				continue
			}

			message := &sarama.ProducerMessage{
				Topic: k.topic,
				Key:   sarama.StringEncoder("log"),
				Value: sarama.ByteEncoder(payload),
			}

			k.producer.Input() <- message

		case <-k.quitChan:
			return
		}
	}
}

// handleProducerErrors processes producer errors.
func (k *KafkaHandler) handleProducerErrors() {
	defer k.wg.Done()
	for {
		select {
		case err, ok := <-k.producer.Errors():
			if !ok {
				return
			}
			fmt.Printf("failed to write message to kafka: %v\n", err)
		case <-k.quitChan:
			return
		}
	}
}

// Enabled checks if the level is enabled.
func (k *KafkaHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// Handle sends logs into a channel for asynchronous processing.
func (k *KafkaHandler) Handle(ctx context.Context, record slog.Record) error {
	select {
	case k.logChan <- record:
		return nil
	default:
		fmt.Println("log channel is full, dropping log message")
		return nil
	}
}

// WithAttrs adds attributes to the handler.
func (k *KafkaHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return k
}

// WithGroup adds a group to the handler.
func (k *KafkaHandler) WithGroup(name string) slog.Handler {
	return k
}

// Close gracefully shuts down KafkaHandler.
func (k *KafkaHandler) Close() error {
	close(k.quitChan)
	k.wg.Wait()
	if err := k.producer.Close(); err != nil {
		return fmt.Errorf("failed to close producer: %w", err)
	}
	return nil
}

// FileHandler saves logs to a file asynchronously.
type FileHandler struct {
	file     *os.File
	logChan  chan slog.Record
	wg       sync.WaitGroup
	quitChan chan struct{}
}

// NewFileHandler initializes a new FileHandler.
func NewFileHandler(serviceName string, bufferSize int) (*FileHandler, error) {
	logDir := filepath.Join("logs", serviceName)
	err := os.MkdirAll(logDir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	logFilePath := filepath.Join(logDir, "app.log")
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	handler := &FileHandler{
		file:     file,
		logChan:  make(chan slog.Record, bufferSize),
		quitChan: make(chan struct{}),
	}

	handler.wg.Add(1)
	go handler.processLogs()

	return handler, nil
}

// processLogs reads log records from a channel and writes them to the file.
func (f *FileHandler) processLogs() {
	defer f.wg.Done()
	for {
		select {
		case record := <-f.logChan:
			line := fmt.Sprintf("[%s] - %s - %s", record.Level.String(), record.Time.Format(time.RFC3339), record.Message)
			f.file.Write(append([]byte(line), '\n'))
		case <-f.quitChan:
			return
		}
	}
}

// Enabled checks if the level is enabled.
func (f *FileHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// Handle sends logs into a channel for asynchronous processing.
func (f *FileHandler) Handle(ctx context.Context, record slog.Record) error {
	select {
	case f.logChan <- record:
		return nil
	default:
		fmt.Println("file log channel is full, dropping log message")
		return nil
	}
}

// WithAttrs adds attributes to the handler.
func (f *FileHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return f
}

// WithGroup adds a group to the handler.
func (f *FileHandler) WithGroup(name string) slog.Handler {
	return f
}

// Close gracefully shuts down FileHandler.
func (f *FileHandler) Close() error {
	close(f.quitChan)
	f.wg.Wait()
	return f.file.Close()
}

// StdoutHandler sends logs to stdout with colored text synchronously.
type StdoutHandler struct {
	writer *os.File
}

// NewStdoutHandler initializes a new StdoutHandler.
func NewStdoutHandler() *StdoutHandler {
	return &StdoutHandler{
		writer: os.Stdout,
	}
}

// Enabled checks if the level is enabled.
func (s *StdoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// Handle processes and outputs the log record to stdout with colors synchronously.
func (s *StdoutHandler) Handle(ctx context.Context, record slog.Record) error {
	color := ColorReset
	switch record.Level {
	case slog.LevelDebug:
		color = ColorBlue
	case slog.LevelInfo:
		color = ColorGreen
	case slog.LevelWarn:
		color = ColorYellow
	case slog.LevelError:
		color = ColorRed
	default:
		color = ColorReset
	}
	line := fmt.Sprintf("%s[%s]%s - %s - %s\n",
		color,
		record.Level.String(),
		ColorReset,
		record.Time.Format("2006-01-02 15:04:05"),
		record.Message,
	)
	_, err := s.writer.Write([]byte(line))
	return err
}

// WithAttrs adds attributes to the handler.
func (s *StdoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s
}

// WithGroup adds a group to the handler.
func (s *StdoutHandler) WithGroup(name string) slog.Handler {
	return s
}

// Close is a no-op for synchronous handler.
func (s *StdoutHandler) Close() error {
	return nil
}

// MultiHandler combines multiple handlers.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler initializes a new MultiHandler.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// Enabled checks if the level is enabled for any handler.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle adds the record to all handlers.
func (m *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if err := h.Handle(ctx, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WithAttrs adds attributes to all handlers.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return NewMultiHandler(handlers...)
}

// WithGroup adds a group to all handlers.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return NewMultiHandler(handlers...)
}

// CloseAll closes all handlers that implement the Close method.
func (m *MultiHandler) CloseAll() {
	for _, h := range m.handlers {
		if closer, ok := h.(interface{ Close() error }); ok {
			closer.Close()
		}
	}
}

// NewLogger initializes the combined logger with Kafka, File, and Stdout handlers.
func NewLogger(brokers []string, kafkaTopic, serviceName string, bufferSize int) (*slog.Logger, error) {
	kafkaHandler, err := NewKafkaHandler(brokers, kafkaTopic, bufferSize)
	if err != nil {
		return nil, err
	}

	fileHandler, err := NewFileHandler(serviceName, bufferSize)
	if err != nil {
		kafkaHandler.Close()
		return nil, err
	}

	stdoutHandler := NewStdoutHandler()

	multiHandler := NewMultiHandler(kafkaHandler, fileHandler, stdoutHandler)

	logger := slog.New(multiHandler)

	return logger, nil
}
