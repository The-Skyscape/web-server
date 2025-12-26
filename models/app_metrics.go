package models

import (
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

type AppMetrics struct {
	application.Model
	AppID     string // legacy - for App metrics
	ProjectID string // new - for Project metrics

	// Container status
	ContainerStatus string // "running", "stopped", "error"
	ReplicaCount    int

	// Resource usage
	CPUUsagePercent float64
	MemoryUsedMB    int64
	MemoryLimitMB   int64

	// Volume storage
	VolumeUsedGB  float64
	VolumeTotalGB float64
	VolumeUsedPct int

	// Timestamps
	LastCheckAt time.Time
}

func (*AppMetrics) Table() string { return "app_metrics" }

func (m *AppMetrics) App() *App {
	if m.AppID == "" {
		return nil
	}
	app, _ := Apps.Get(m.AppID)
	return app
}

func (m *AppMetrics) Project() *Project {
	if m.ProjectID == "" {
		return nil
	}
	project, _ := Projects.Get(m.ProjectID)
	return project
}
