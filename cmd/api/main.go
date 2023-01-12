package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/tmaxmax/go-sse"
	"html/template"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

type config struct {
	serverPort       int
	noFlyZoneOriginX float64
	noFlyZoneOriginY float64
	noFlyZoneRadius  float64
	sleepDuration    time.Duration
	persistDuration  time.Duration
}

type application struct {
	sseHandler *sse.Server
	cfg        config
	tmpl       *template.Template
	homepage   []byte
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func main() {
	var cfg config

	flag.IntVar(&cfg.serverPort, "port", getEnvInt("PORT", 8080), "API server port")
	var (
		sleepDuration   int
		persistDuration int
	)
	flag.IntVar(&sleepDuration, "sleep", 2000, "Timeout between drone position polls (milliseconds)")
	flag.IntVar(&persistDuration, "persist", 10, "Time to persist violating pilots (minutes)")
	flag.Float64Var(&cfg.noFlyZoneRadius, "no-fly-zone-radius", 100, "Radius of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginX, "no-fly-zone-origin-x", 250000, "Origin X coordinate of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginY, "no-fly-zone-origin-y", 250000, "Origin Y coordinate of no-fly zone in meters")

	flag.Parse()
	cfg.sleepDuration = time.Duration(sleepDuration) * time.Millisecond
	cfg.persistDuration = time.Duration(persistDuration) * time.Minute

	tmpl, err := template.ParseGlob("./ui/html/*")
	if err != nil {
		panic("failed to read templates")
	}

	app := &application{
		sseHandler: sse.NewServer(),
		cfg:        cfg,
		tmpl:       tmpl,
	}

	go app.monitor()
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.serverPort), app.routes())
}

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", app.sseHandler.ServeHTTP)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(app.homepage)
	})

	return mux
}

type Report struct {
	XMLName           xml.Name `xml:"report"`
	Text              string   `xml:",chardata"`
	DeviceInformation struct {
		Text             string `xml:",chardata"`
		DeviceId         string `xml:"deviceId,attr"`
		ListenRange      string `xml:"listenRange"`
		DeviceStarted    string `xml:"deviceStarted"`
		UptimeSeconds    string `xml:"uptimeSeconds"`
		UpdateIntervalMs string `xml:"updateIntervalMs"`
	} `xml:"deviceInformation"`
	Capture struct {
		Text              string    `xml:",chardata"`
		SnapshotTimestamp time.Time `xml:"snapshotTimestamp,attr"`
		Drone             []Drone   `xml:"drone"`
	} `xml:"capture"`
}

type Drone struct {
	Text         string  `xml:",chardata"`
	SerialNumber string  `xml:"serialNumber"`
	Model        string  `xml:"model"`
	Manufacturer string  `xml:"manufacturer"`
	Mac          string  `xml:"mac"`
	Ipv4         string  `xml:"ipv4"`
	Ipv6         string  `xml:"ipv6"`
	Firmware     string  `xml:"firmware"`
	PositionY    float64 `xml:"positionY"`
	PositionX    float64 `xml:"positionX"`
	Altitude     float64 `xml:"altitude"`
}

type Pilot struct {
	PilotID     string    `json:"pilotId"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	PhoneNumber string    `json:"phoneNumber"`
	CreatedDt   time.Time `json:"createdDt"`
	Email       string    `json:"email"`
}

type Violation struct {
	Pilot           Pilot
	LastTime        time.Time
	ClosestDistance float64
}

type templateData struct {
	Violations []Violation
}

// TODO: use timer
func (app *application) monitor() {
	cache := make(map[string]Violation)

	for {
		time.Sleep(app.cfg.sleepDuration)

		resp, err := http.Get("https://assignments.reaktor.com/birdnest/drones")
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var report Report
		err = xml.Unmarshal(body, &report)
		if err != nil {
			continue
		}

		for _, drone := range report.Capture.Drone {
			distance := math.Hypot(app.cfg.noFlyZoneOriginX-drone.PositionX, app.cfg.noFlyZoneOriginY-drone.PositionY) / 1000
			if distance > app.cfg.noFlyZoneRadius {
				continue
			}

			// Check if violation entry exists already
			if violation, ok := cache[drone.SerialNumber]; ok {
				if distance < violation.ClosestDistance {
					violation.ClosestDistance = distance
				}

				violation.LastTime = report.Capture.SnapshotTimestamp

				cache[drone.SerialNumber] = violation
			} else {
				resp, err = http.Get(fmt.Sprintf("https://assignments.reaktor.com/birdnest/pilots/%s", drone.SerialNumber))
				if err != nil {
					continue
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					continue
				}

				var pilot Pilot
				err = json.Unmarshal(body, &pilot)
				if err != nil {
					continue
				}

				cache[drone.SerialNumber] = Violation{
					Pilot:           pilot,
					LastTime:        report.Capture.SnapshotTimestamp,
					ClosestDistance: distance,
				}
			}
		}

		// Delete old entries
		for drone, violation := range cache {
			if report.Capture.SnapshotTimestamp.Sub(violation.LastTime) > app.cfg.persistDuration {
				delete(cache, drone)
			}
		}

		var violations []Violation
		for _, violation := range cache {
			violations = append(violations, violation)
		}

		sort.Slice(violations, func(i, j int) bool {
			return violations[i].LastTime.After(violations[j].LastTime)
		})

		td := &templateData{
			Violations: violations,
		}

		homeBuf := new(bytes.Buffer)
		app.tmpl.ExecuteTemplate(homeBuf, "home", td)
		app.homepage = homeBuf.Bytes()

		pilotBuf := new(bytes.Buffer)
		app.tmpl.ExecuteTemplate(pilotBuf, "pilot", td)

		e := &sse.Message{}
		e.AppendData(pilotBuf.Bytes())
		app.sseHandler.Publish(e)

	}
}
