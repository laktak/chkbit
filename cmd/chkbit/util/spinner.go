package util

import (
	"time"
)

func Spinner(timeout time.Duration) <-chan string {
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ch := make(chan string)
	go func() {
		for i := 0; ; i++ {
			ch <- spinnerChars[i%len(spinnerChars)]
			time.Sleep(timeout)
		}
	}()
	return ch
}
