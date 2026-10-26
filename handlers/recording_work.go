package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

var errRecordingBusy = errors.New("recording processing capacity is busy; please retry")

func recordingConcurrency() int {
	value, err := strconv.Atoi(os.Getenv("FDM_RECORDING_CONCURRENCY"))
	if err != nil || value < 1 || value > 32 {
		return 2
	}
	return value
}

// Reject excess work instead of retaining an unbounded queue of uploaded
// recordings and waiting requests. Ordinary database reads do not use slots.
func tryRecordingWork(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func acquireRecordingWork(c *gin.Context, slots chan struct{}) bool {
	if c.Request.Context().Err() != nil {
		return false
	}
	if tryRecordingWork(slots) {
		return true
	}
	respondRecordingBusy(c)
	return false
}

func releaseRecordingWork(slots chan struct{}) { <-slots }

func respondRecordingBusy(c *gin.Context) {
	c.Header("Retry-After", "2")
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": errRecordingBusy.Error(), "code": "RECORDING_BUSY"})
}
