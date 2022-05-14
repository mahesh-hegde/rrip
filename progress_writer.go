package main

import (
	"io"
	"time"
)

// ProgressWriter wraps a io.Writer and an updater callback
// It calls updater callback only if there are more than 1024 bytes written
// And more than 500ms has elapsed since last call to callback

const significantWrite = 1024 * 10
const significantTime = 500

type ProgressWriter struct {
	Callback  func(int64)
	Writer    io.Writer
	total     int64
	lastCall  int64
	lastTotal int64
}

func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.Writer.Write(p)
	pw.total += int64(n)
	var writeCondition, timeCondition bool
	var now time.Time
	writeCondition = (pw.total - pw.lastTotal) > significantWrite
	if writeCondition {
		now = time.Now()
		timeCondition = pw.lastCall == 0 ||
			(now.UnixMilli()-pw.lastCall) > significantTime
	}
	if writeCondition && timeCondition {
		pw.lastCall = now.UnixMilli()
		pw.lastTotal = pw.total
		pw.Callback(pw.total)
	}
	return n, err
}
