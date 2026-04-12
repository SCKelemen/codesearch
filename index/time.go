package index

import "time"

func init() {
	timeNow = func() int64 {
		return time.Now().UnixMilli()
	}
}
