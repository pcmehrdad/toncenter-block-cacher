package utils

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

type Logger struct {
	mu          sync.Mutex
	lastLogTime map[string]time.Time
	minInterval time.Duration
}

func NewLogger() *Logger {
	return &Logger{
		lastLogTime: make(map[string]time.Time),
		minInterval: time.Second * 5, // Log same type of message at most every 5 seconds
	}
}

func (l *Logger) Info(category string, message string, data map[string]interface{}) {
	l.mu.Lock()
	lastTime, exists := l.lastLogTime[category]
	now := time.Now()

	// Only log if enough time has passed since last similar log
	if !exists || now.Sub(lastTime) >= l.minInterval {
		l.lastLogTime[category] = now
		l.mu.Unlock()

		logEntry := map[string]interface{}{
			"time":     now.Format(time.RFC3339),
			"level":    "INFO",
			"category": category,
			"message":  message,
		}

		if data != nil {
			for k, v := range data {
				logEntry[k] = v
			}
		}

		jsonLog, _ := json.Marshal(logEntry)
		log.Println(string(jsonLog))
	} else {
		l.mu.Unlock()
	}
}

func (l *Logger) Error(category string, message string, err error, data map[string]interface{}) {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["error"] = err.Error()

	logEntry := map[string]interface{}{
		"time":     time.Now().Format(time.RFC3339),
		"level":    "ERROR",
		"category": category,
		"message":  message,
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonLog, _ := json.Marshal(logEntry)
	log.Println(string(jsonLog))
}
