package main

import (
	"fmt"
	"math"
	"reaktor-birdnest/internal/models"
	"sync"
	"time"
)

func (app *application) monitor(done <-chan bool, dispatchViolations func([]models.Violation)) {
	ticker := time.NewTicker(app.cfg.sleepDuration)
	for {
		select {
		case <-done:
			ticker.Stop()
			app.violations.Destroy()
			return
		case <-ticker.C:
			report, err := app.birdnest.GetReport()
			if err != nil {
				fmt.Println(err)
			}

			wg := sync.WaitGroup{}
			for _, drone := range report.Capture.Drone {
				// Capture variable for goroutine
				drone := drone

				wg.Add(1)
				go func() {
					defer wg.Done()
					distance := math.Hypot(app.cfg.noFlyZoneOriginX-drone.PositionX, app.cfg.noFlyZoneOriginY-drone.PositionY)
					// Convert millimeters to meters
					distance /= 1000
					if distance > app.cfg.noFlyZoneRadius {
						return
					}

					// Check if violation entry exists already
					violation, found := app.violations.Get(drone.SerialNumber)
					if found {
						if distance < violation.ClosestDistance {
							violation.ClosestDistance = distance
						}
					} else {
						pilot, err := app.birdnest.GetDronePilot(drone.SerialNumber)
						if err != nil {
							fmt.Println(err)
							return
						}

						violation = models.Violation{
							Pilot:           pilot,
							ClosestDistance: distance,
						}
					}

					app.violations.Upsert(drone.SerialNumber, violation)
				}()
			}
			wg.Wait()

			// Try to send new event only when something has changed
			if app.violations.HasChanges() {
				dispatchViolations(app.violations.AsSlice())
			}
		default:
			continue
		}
	}
}
