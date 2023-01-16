package main

import (
	"encoding/xml"
	"errors"
	"reaktor-birdnest/internal/models"
	"reaktor-birdnest/internal/persistance/datastore"
	"strings"
	"testing"
	"time"
)

func TestAddingViolations(t *testing.T) {
	app := newApp()
	app.violations = datastore.New[models.Violation](app.cfg.persistDuration)

	expectedDistance := 50.0
	violations := runMonitor(&app, &BirdnestMock{
		drones: [][]DronePartial{
			{
				{
					SerialNumber: "123",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + expectedDistance*1000,
				},
				{
					SerialNumber: "456",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + 10000000000000,
				},
			},
		},
		pilots: map[string]models.Pilot{
			"123": testingPilot("Bob"),
			"456": testingPilot("Billy"),
		},
	})

	if len(violations) != 1 {
		t.Errorf("Expected violations to be of length 1, but was %d.", len(violations))
	}

	first := violations[0]
	if len(first) != 1 {
		t.Errorf("Expected first to be of length 1, but was %d.", len(first))
	}

	violation := first[0]
	if strings.Compare(violation.Pilot.FirstName, "Bob") != 0 {
		t.Errorf("Expected violation pilot to be 'Bob', but was '%s'.", violation.Pilot.FirstName)
	}

	if !almostEquals(violation.ClosestDistance, expectedDistance, 0.001) {
		t.Errorf("Expected closest distance to be %f, but was %f.", expectedDistance, violation.ClosestDistance)
	}
}

func TestRemoval(t *testing.T) {
	app := newApp()
	// Set duration to zero violations are removed in the next tick
	app.cfg.persistDuration = 0
	app.violations = datastore.New[models.Violation](app.cfg.persistDuration)

	expectedDistance := 50.0
	violations := runMonitor(&app, &BirdnestMock{
		drones: [][]DronePartial{
			{
				{
					SerialNumber: "123",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + expectedDistance*1000,
				},
			},
		},
		pilots: map[string]models.Pilot{
			"123": testingPilot("Bob"),
		},
	})

	if len(violations) != 2 {
		t.Errorf("Expected violations to be of length 2, but was %d.", len(violations))
	}

	bob := violations[0]
	if len(bob) != 1 {
		t.Errorf("Expected bob to be of length 1, but was %d.", len(bob))
	}

	if strings.Compare(bob[0].Pilot.FirstName, "Bob") != 0 {
		t.Errorf("Expected bob[0] pilot to be 'Bob', but was '%s'.", bob[0].Pilot.FirstName)
	}

	empty := violations[1]
	if len(empty) != 0 {
		t.Errorf("Expected empty to be of length 0, but was %d.", len(empty))
	}
}

func TestUpdateExistingPilot(t *testing.T) {
	app := newApp()
	app.violations = datastore.New[models.Violation](app.cfg.persistDuration)

	firstDistance := 50.0
	secondDistance := 40.0

	violations := runMonitor(&app, &BirdnestMock{
		drones: [][]DronePartial{
			{
				{
					SerialNumber: "123",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + firstDistance*1000,
				},
			},
			{
				{
					SerialNumber: "456",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + firstDistance*1000,
				},
			},
			{
				{
					SerialNumber: "123",
					PositionY:    app.cfg.noFlyZoneOriginY,
					PositionX:    app.cfg.noFlyZoneOriginX + secondDistance*1000,
				},
			},
		},
		pilots: map[string]models.Pilot{
			"123": testingPilot("Bob"),
			"456": testingPilot("Billy"),
		},
	})

	if len(violations) != 3 {
		t.Errorf("Expected violations to be of length 2, but was %d.", len(violations))
	}

	last := violations[2]
	if len(last) != 2 {
		t.Errorf("Expected last to be of length 2, but was %d.", len(last))
	}

	bob := last[0]
	billy := last[1]

	if strings.Compare(bob.Pilot.FirstName, "Bob") != 0 {
		t.Errorf("Expected bob pilots name to be 'Bob', but was '%s'.", bob.Pilot.FirstName)
	}

	if !almostEquals(bob.ClosestDistance, secondDistance, 0.001) {
		t.Errorf("Expected closest distance of bob to be %f, but was %f.", secondDistance, bob.ClosestDistance)
	}

	if strings.Compare(billy.Pilot.FirstName, "Billy") != 0 {
		t.Errorf("Expected bob pilots name to be 'Billy', but was '%s'.", billy.Pilot.FirstName)
	}

	if !almostEquals(billy.ClosestDistance, firstDistance, 0.001) {
		t.Errorf("Expected closest distance of billy to be %f, but was %f.", firstDistance, billy.ClosestDistance)
	}
}

func runMonitor(app *application, birdnest *BirdnestMock) [][]models.Violation {
	done := make(chan bool, 1)
	violations := make([][]models.Violation, 0)
	birdnest.end = done
	app.birdnest = birdnest

	app.monitor(done, func(v []models.Violation) {
		violations = append(violations, v)
		// Give time for expiring
		time.Sleep(2 * time.Millisecond)
	})
	return violations
}

func testingPilot(firstName string) models.Pilot {
	return models.Pilot{
		PilotID:     "123",
		FirstName:   firstName,
		LastName:    "Tester",
		PhoneNumber: "+123",
		CreatedDt:   time.Now().UTC(),
		Email:       firstName + "@email.com",
	}
}

func newApp() application {
	return application{
		cfg: config{
			noFlyZoneOriginX: 250000,
			noFlyZoneOriginY: 250000,
			noFlyZoneRadius:  100,
			sleepDuration:    time.Millisecond,
			persistDuration:  10 * time.Minute,
		},
	}
}

type DronePartial struct {
	SerialNumber string
	PositionY    float64
	PositionX    float64
}

type BirdnestMock struct {
	end     chan<- bool
	drones  [][]DronePartial
	pilots  map[string]models.Pilot
	current int
}

func (b *BirdnestMock) GetReport() (models.Report, error) {
	if b.current == len(b.drones) {
		b.end <- true
		return models.Report{}, errors.New("end")
	}

	drones := make([]models.Drone, 0, len(b.drones))
	for _, dp := range b.drones[b.current] {
		drones = append(drones, models.Drone{
			Text:         "",
			SerialNumber: dp.SerialNumber,
			Model:        "",
			Manufacturer: "",
			Mac:          "",
			Ipv4:         "",
			Ipv6:         "",
			Firmware:     "",
			PositionY:    dp.PositionY,
			PositionX:    dp.PositionX,
			Altitude:     0,
		})
	}

	b.current++

	return models.Report{
		XMLName: xml.Name{
			Space: "",
			Local: "",
		},
		Text: "",
		DeviceInformation: struct {
			Text             string `xml:",chardata"`
			DeviceId         string `xml:"deviceId,attr"`
			ListenRange      string `xml:"listenRange"`
			DeviceStarted    string `xml:"deviceStarted"`
			UptimeSeconds    string `xml:"uptimeSeconds"`
			UpdateIntervalMs string `xml:"updateIntervalMs"`
		}{
			Text:             "",
			DeviceId:         "",
			ListenRange:      "",
			DeviceStarted:    "",
			UptimeSeconds:    "",
			UpdateIntervalMs: "",
		},
		Capture: struct {
			Text              string         `xml:",chardata"`
			SnapshotTimestamp time.Time      `xml:"snapshotTimestamp,attr"`
			Drone             []models.Drone `xml:"drone"`
		}{
			Text:              "",
			SnapshotTimestamp: time.Now().UTC(),
			Drone:             drones,
		},
	}, nil
}

func (b *BirdnestMock) GetDronePilot(droneSerialNumber string) (models.Pilot, error) {
	if pilot, ok := b.pilots[droneSerialNumber]; ok {
		return pilot, nil
	}

	panic("pilot not found for " + droneSerialNumber)
}

func almostEquals(a, b, tolerance float64) bool {
	return (a-b) < tolerance && (b-a) < tolerance
}
