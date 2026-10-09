package control

import (
	"runtime"
	"sort"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
)

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
	Components     []components.Descriptor
	Resilience     *ResilienceIngestServer
}

type metadataResponse struct {
	Version      string                  `json:"version"`
	Engine       metadataEngine          `json:"engine"`
	Runtime      metadataRuntime         `json:"runtime"`
	Capabilities []string                `json:"capabilities"`
	Control      metadataControl         `json:"control"`
	Components   []components.Descriptor `json:"components,omitempty"`
	Application  metadataApplication     `json:"application"`
}

type metadataApplication struct {
	Resilience ResilienceSummary `json:"resilience"`
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
	resilience := ResilienceSummary{SchemaVersion: "v1", Status: "unavailable", Policies: []ResiliencePolicy{}, Circuits: []ResilienceCircuitSummary{}}
	if m.Resilience != nil {
		resilience = m.Resilience.Store().Snapshot(time.Now(), !remoteAccess)
	}
	capabilities := append([]string(nil), m.Capabilities...)
	descriptors := make([]components.Descriptor, 0, len(m.Components))
	for _, descriptor := range m.Components {
		copyOf := descriptor
		copyOf.Capabilities = append([]components.Capability(nil), descriptor.Capabilities...)
		copyOf.SecretFields = append([]string(nil), descriptor.SecretFields...)
		copyOf.Schema.Fields = make(map[string]components.FieldSchema, len(descriptor.Schema.Fields))
		for name, field := range descriptor.Schema.Fields {
			copyOf.Schema.Fields[name] = field
		}
		descriptors = append(descriptors, copyOf)
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].Name < descriptors[j].Name })
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
		Components:  descriptors,
		Application: metadataApplication{Resilience: resilience},
	}
}
