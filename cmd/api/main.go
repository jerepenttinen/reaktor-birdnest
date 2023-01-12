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
	"time"
)

type config struct {
	serverPort       int
	noFlyZoneOriginX float64
	noFlyZoneOriginY float64
	noFlyZoneRadius  float64
	sleepDuration    int
}

type application struct {
	sseHandler *sse.Server
	cfg        config
	tmpl       *template.Template
}

func main() {
	var cfg config

	flag.IntVar(&cfg.serverPort, "port", getEnvInt("PORT", 8080), "API server port")
	flag.IntVar(&cfg.sleepDuration, "sleep", 2000, "Timeout between drone position polls (milliseconds)")
	flag.Float64Var(&cfg.noFlyZoneRadius, "no-fly-zone-radius", 100, "Radius of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginX, "no-fly-zone-origin-x", 250000, "Origin X coordinate of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginY, "no-fly-zone-origin-y", 250000, "Origin Y coordinate of no-fly zone in meters")

	flag.Parse()

	// Convert from meters to millimeters
	cfg.noFlyZoneRadius *= 1000

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
		app.tmpl.ExecuteTemplate(w, "home", nil)
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
	pilot           Pilot
	lastTime        time.Time
	closestDistance float64
}

// TODO: use timer
func (app *application) monitor() {
	cache := make(map[string]Violation)

	for {
		time.Sleep(time.Duration(app.cfg.sleepDuration) * time.Millisecond)

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
			distance := math.Hypot(app.cfg.noFlyZoneOriginX-drone.PositionX, app.cfg.noFlyZoneOriginY-drone.PositionY)
			if distance > app.cfg.noFlyZoneRadius {
				continue
			}

			// Check if violation entry exists already
			if violation, ok := cache[drone.SerialNumber]; ok {
				if distance < violation.closestDistance {
					violation.closestDistance = distance
				}

				violation.lastTime = report.Capture.SnapshotTimestamp

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
					pilot:           pilot,
					lastTime:        report.Capture.SnapshotTimestamp,
					closestDistance: distance,
				}
			}
		}

		// Delete old entries
		for drone, violation := range cache {
			if report.Capture.SnapshotTimestamp.Sub(violation.lastTime) > time.Minute*10 {
				delete(cache, drone)
			}
		}

		fmt.Println("\nNEWTICK")
		for drone := range cache {
			fmt.Println(drone)
		}

		buf := new(bytes.Buffer)
		app.tmpl.ExecuteTemplate(buf, "pilot", nil)

		e := &sse.Message{}
		e.AppendData(buf.Bytes())
		app.sseHandler.Publish(e)
	}
}
