package plan

import "time"

type Plan struct {
	ID              string
	Name            string
	CPURequestM     int
	CPULimitM       int
	MemoryRequestMi int
	MemoryLimitMi   int
	Replicas        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
