package control

import "runtime"

type Metadata struct {
	EngineName     string
	Version        string
	GoVersion      string
	OS             string
	Arch           string
	WorkerCount    int
	TimeoutSeconds int
	Capabilities   []string
	RemoteAccess   bool
}

type metadataResponse struct {
	Version      string          `json:"version"`
	Engine       metadataEngine  `json:"engine"`
	Runtime      metadataRuntime `json:"runtime"`
	Capabilities []string        `json:"capabilities"`
	Control      metadataControl `json:"control"`
}

type metadataEngine struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type metadataRuntime struct {
	WorkerCount    int `json:"worker_count"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

type metadataControl struct {
	RemoteAccess bool `json:"remote_access"`
}

func (m Metadata) safeResponse(remoteAccess bool) metadataResponse {
	capabilities := append([]string(nil), m.Capabilities...)
	goVersion := m.GoVersion
	if goVersion == "" {
		goVersion = runtime.Version()
	}

	return metadataResponse{
		Version: "v1",
		Engine: metadataEngine{
			Name:      m.EngineName,
			Version:   m.Version,
			GoVersion: goVersion,
			OS:        m.OS,
			Arch:      m.Arch,
		},
		Runtime: metadataRuntime{
			WorkerCount:    m.WorkerCount,
			TimeoutSeconds: m.TimeoutSeconds,
		},
		Capabilities: capabilities,
		Control: metadataControl{
			RemoteAccess: remoteAccess,
		},
	}
}
