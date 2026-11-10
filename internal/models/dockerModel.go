package models

type DockerModel struct {
	ID     string
	Name   string
	Ports  string // https://pkg.go.dev/github.com/moby/moby/api/types/container#PortSummary
	State  string //https://pkg.go.dev/github.com/moby/moby/api/types/container#ContainerState
	Status string // https://pkg.go.dev/github.com/moby/moby/api/types/container#Summary
	Health string //https://pkg.go.dev/github.com/moby/moby/api/types/container#HealthStatus
	//ContainerCPU      float64
	//ContainerMemUsage float64
	//ContainerLimitMem    float64
}
