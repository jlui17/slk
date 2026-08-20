package thread

// SetPendingSelectTS pins the cursor to ts across the SetThread
// reloads of the current thread — see the pendingSelectTS field. Arm
// it after the SetThread that opens the thread: SetThread clears the
// pin on a thread-identity change, so arming first would drop it.
func (m *Model) SetPendingSelectTS(ts string) {
	m.pendingSelectTS = ts
}

// SelectByTS moves the selection cursor to the reply with the given
// ts, or to the parent row when ts is the parent's. Returns false
// (cursor untouched) when ts matches neither.
func (m *Model) SelectByTS(ts string) bool {
	if ts == "" {
		return false
	}
	target := parentSelected - 1
	if ts == m.parent.TS {
		target = parentSelected
	} else {
		for i := range m.replies {
			if m.replies[i].TS == ts {
				target = i
				break
			}
		}
	}
	if target < parentSelected {
		return false
	}
	m.selected = target
	m.hasSnapped = false
	m.viewCacheValid = false
	m.dirty()
	return true
}
