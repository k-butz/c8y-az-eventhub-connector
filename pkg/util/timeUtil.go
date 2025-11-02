package util

import (
	"time"
)

const TS_FORMAT string = "2006-01-02T15:04:05.000Z07:00"

func ToRFCTimeStamp(t time.Time) string {
	return t.Format(TS_FORMAT)
}
