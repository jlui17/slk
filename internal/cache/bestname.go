package cache

// BestName returns DisplayName, falling back to Name.
func (u User) BestName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Name
}
