package clock

import "time"

// Clock abstracts time for deterministic tests and strict UTC usage.
type Clock interface {
	NowUTC() time.Time
}

// SystemUTC is the production clock.
type SystemUTC struct{}

func (SystemUTC) NowUTC() time.Time {
	return time.Now().UTC()
}
