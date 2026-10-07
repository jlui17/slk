package thread

// SetWorkspaceDomain sets the subdomain user mentions link into.
func (m *Model) SetWorkspaceDomain(domain string) {
	if m.workspaceDomain == domain {
		return
	}
	m.workspaceDomain = domain
	m.InvalidateCache()
}
