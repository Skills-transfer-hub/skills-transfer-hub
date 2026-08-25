package timing

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	markPathOnce sync.Once
	markPath     string
	processStart = time.Now()
)

func Mark(step string) {
	markPathOnce.Do(func() {
		markPath = os.Getenv("STH_TIMING")
	})
	if markPath == "" {
		return
	}
	file, err := os.OpenFile(markPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(file, "%dms\t%s\n", time.Since(processStart).Milliseconds(), step)
	file.Close()
}
