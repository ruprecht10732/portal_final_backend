package tools

func IsKnownTool(name string) bool {
	if _, ok := domainToolNames[name]; ok {
		return true
	}
	_, ok := systemToolNames[name]
	return ok
}
