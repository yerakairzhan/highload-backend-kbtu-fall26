package database

import (
	"sync"
	"time"
)

// snowflakeEpoch is 2026-01-01T00:00:00Z: 41 bits of milliseconds last ~69 years from here.
var snowflakeEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// Snowflake makes 64-bit ids: 41 bits of milliseconds | 10 bits worker | 12 bits sequence.
// They sort by creation time, so an id doubles as a pagination cursor.
type Snowflake struct {
	mu     sync.Mutex
	worker uint64
	lastMs int64
	seq    uint64
}

var ids = NewSnowflake(1)

func NewSnowflake(worker uint64) *Snowflake {
	return &Snowflake{worker: worker & 0x3FF}
}

func (s *Snowflake) Next() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms := time.Now().UnixMilli() - snowflakeEpoch
	if ms < s.lastMs {
		ms = s.lastMs // clock went backwards: stay monotonic
	}
	if ms == s.lastMs {
		s.seq = (s.seq + 1) & 0xFFF
		if s.seq == 0 { // 4 096 ids this millisecond: borrow the next one
			ms++
		}
	} else {
		s.seq = 0
	}
	s.lastMs = ms
	return uint64(ms)<<22 | s.worker<<12 | s.seq
}

// NextId plays the role of the database's id default.
func NextId() uint64 {
	return ids.Next()
}
