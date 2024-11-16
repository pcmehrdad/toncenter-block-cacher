package utils

import (
	"time"
)

func WithRetry(attempts int, delay time.Duration, fn func() error) error {
	var err error

	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}

		if err = fn(); err == nil {
			return nil
		}
	}

	return err
}
