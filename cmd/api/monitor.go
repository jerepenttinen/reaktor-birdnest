package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

func (app *application) monitor(done <-chan bool, dispatchViolations func([]Violation)) {
	ticker := time.NewTicker(app.cfg.sleepDuration)
	for {
		select {
		case <-done:
			ticker.Stop()
			return
		case <-ticker.C:
			report, err := app.birdnest.GetReport()
			if err != nil {
				fmt.Println(err)
			}

			currentTime := time.Now().UTC()

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

						violation.LastTime = currentTime
					} else {
						pilot, err := app.birdnest.GetDronePilot(drone.SerialNumber)
						if err != nil {
							fmt.Println(err)
							return
						}

						violation = Violation{
							Pilot:           pilot,
							LastTime:        currentTime,
							ClosestDistance: distance,
						}
					}

					app.violations.Upsert(drone.SerialNumber, violation)
				}()
			}
			wg.Wait()

			app.violations.DeleteOldestWhile(func(v Violation) bool {
				return currentTime.Sub(v.LastTime) > app.cfg.persistDuration
			})

			// Try to send new event only when something has changed
			if app.violations.HasChanges() {
				dispatchViolations(app.violations.AsSlice())
			}
		default:
			continue
		}
	}
}
