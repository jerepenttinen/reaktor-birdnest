package interfaces

import "reaktor-birdnest/internal/models"

type Birdnest interface {
	GetReport() (models.Report, error)
	GetDronePilot(droneSerialNumber string) (models.Pilot, error)
}

type Violations interface {
	Get(id string) (models.Violation, bool)
	Upsert(id string, data models.Violation)
	Destroy()
	AsSlice() []models.Violation
	HasChanges() bool
}
