package coreservice

type Registration struct {
	Backend      string `json:"backend"`
	Name         string `json:"name"`
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	Executable   string `json:"executable,omitempty"`
	LegacyHelper bool   `json:"legacy_helper,omitempty"`
	Error        string `json:"error,omitempty"`
}
