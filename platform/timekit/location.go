package timekit

import "time"

// ResolveLocation returns a non-nil location for time formatting.
// It prefers the requested IANA timezone, then the process local timezone,
// and finally UTC when timezone data is unavailable.
func ResolveLocation(name string) *time.Location {
	if loc, err := time.LoadLocation(name); err == nil && loc != nil {
		return loc
	}
	if time.Local != nil {
		return time.Local
	}
	return time.UTC
}
