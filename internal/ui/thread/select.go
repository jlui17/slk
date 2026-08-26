package thread

// SetPendingSelectTS arms a one-shot cursor pin for a permalink open:
// the next SetThread of the current thread whose content contains ts
// selects it, then the pin disarms so later reloads (reconnect
// refetches) can't yank the cursor back. Arm it after the SetThread
// that opens the thread: SetThread drops the pin on a thread-identity
// change, so arming first would lose it.
func (m *Model) SetPendingSelectTS(ts string) {
	m.pendingSelectTS = ts
}

// selectionSnapshot captures where the cursor sat before SetThread
// replaced the panel's content, so a reload of the same thread can put
// it back. The zero value means the reload was a thread switch: the
// default newest-reply cursor stands.
type selectionSnapshot struct {
	sameThread bool
	onParent   bool
	ts         string
	hasSnapped bool
}

// snapshotSelection is SetThread's entry hook, called before any state
// is replaced. On a thread-identity change it drops a stale
// pending-select pin and captures nothing.
func (m *Model) snapshotSelection(channelID, threadTS string) selectionSnapshot {
	if channelID != m.channelID || threadTS != m.threadTS {
		m.pendingSelectTS = ""
		return selectionSnapshot{}
	}
	s := selectionSnapshot{sameThread: true, hasSnapped: m.hasSnapped}
	if m.selected == parentSelected {
		s.onParent = true
	} else if m.selected >= 0 && m.selected < len(m.replies) {
		s.ts = m.replies[m.selected].TS
	}
	return s
}

// restoreSelection is SetThread's exit hook, re-applying the cursor
// after the newest-reply default: an armed pending-select pin wins and
// disarms once it lands; otherwise a same-thread reload puts the cursor
// back on the message it was on, arming the pin with that ts when the
// pass doesn't contain it (so a stub reload doesn't lose the place for
// the passes after it). It also keeps the snap state, so the viewport
// stays where the user scrolled it — View() still re-snaps whenever the
// cursor actually moved (snappedSelection != selected).
func (m *Model) restoreSelection(prev selectionSnapshot) {
	if m.pendingSelectTS != "" && m.SelectByTS(m.pendingSelectTS) {
		m.pendingSelectTS = ""
		return
	}
	if !prev.sameThread {
		return
	}
	if prev.onParent {
		m.selected = parentSelected
	} else if prev.ts != "" && !m.SelectByTS(prev.ts) {
		// This pass doesn't contain the cursor's message (a stub or
		// truncated-cache reload, or the message was deleted): leave the
		// newest-reply default and arm the pin so the pass that does
		// contain it puts the cursor back.
		m.pendingSelectTS = prev.ts
	}
	m.hasSnapped = prev.hasSnapped
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
