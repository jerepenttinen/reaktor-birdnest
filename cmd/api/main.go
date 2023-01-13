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
	sseHandler    *sse.Server
	cfg           config
	tmpl          *template.Template
	homepage      []byte
	homepageMutex sync.RWMutex
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
		app.homepageMutex.RLock()
		defer app.homepageMutex.RUnlock()
		w.Write(app.homepage)
	})

	return mux
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
		report, err := data.GetReport()
		if err != nil {
			fmt.Println(err)
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
				pilot, err := data.GetDronePilot(drone.SerialNumber)
				if err != nil {
					fmt.Println(err)
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

		app.homepageMutex.Lock()
		app.homepage = homeBuf.Bytes()
		app.homepageMutex.Unlock()

		pilotBuf := new(bytes.Buffer)
		app.tmpl.ExecuteTemplate(pilotBuf, "pilot", td)

		e := &sse.Message{}
		e.AppendData(pilotBuf.Bytes())
		app.sseHandler.Publish(e)

	}
}
