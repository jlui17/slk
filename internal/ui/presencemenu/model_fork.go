package presencemenu

import "time"

func (m *Model) now() time.Time {
	if m.nowFn != nil {
		return m.nowFn()
	}
	return time.Now()
}

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (m *Model) SetNowFunc(fn func() time.Time) { m.nowFn = fn }
