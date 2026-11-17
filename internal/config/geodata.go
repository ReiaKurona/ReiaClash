package config

func GeodataLoader(lowMemory bool) string {
	if lowMemory {
		return "memconservative"
	}
	return "standard"
}
