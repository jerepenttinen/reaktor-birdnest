package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/tmaxmax/go-sse"
	"html/template"
	"math"
	"net/http"
	"os"
	reaktorbirdnest "reaktor-birdnest"
	"reaktor-birdnest/internal/datastore"
	"reaktor-birdnest/internal/models"
	"reaktor-birdnest/internal/models/birdnest"
	"strconv"
	"sync"
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
	birdnest      interface {
		GetReport() (models.Report, error)
		GetDronePilot(droneSerialNumber string) (models.Pilot, error)
	}
	violations interface {
		Get(id string) (*Violation, bool)
		Upsert(id string, data *Violation)
		DeleteOldestWhile(cond func(violation Violation) bool)
		AsSlice() []Violation
		HasChanges() bool
	}
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

	tmpl, err := template.ParseFS(reaktorbirdnest.TemplateFS, "ui/html/*")
	if err != nil {
		panic("failed to read templates")
	}

	app := &application{
		sseHandler: sse.NewServer(),
		cfg:        cfg,
		tmpl:       tmpl,
		birdnest:   birdnest.Birdnest{},
		violations: datastore.New[Violation](),
	}

	go app.monitor(make(chan bool), app.processViolations)
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
	Pilot           models.Pilot
	LastTime        time.Time
	ClosestDistance float64
}

type templateData struct {
	Violations []Violation
}

func (app *application) monitor(done chan bool, dispatchViolations func([]Violation)) {
	ticker := time.NewTicker(app.cfg.sleepDuration)
	for {
		select {
		case <-done:
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

						violation = &Violation{
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
		}
	}
}

func (app *application) processViolations(violations []Violation) {
	td := &templateData{
		Violations: violations,
	}

	homeBuf := new(bytes.Buffer)
	err := app.tmpl.ExecuteTemplate(homeBuf, "home", td)
	if err == nil {
		app.homepageMutex.Lock()
		app.homepage = homeBuf.Bytes()
		app.homepageMutex.Unlock()
	}

	pilotBuf := new(bytes.Buffer)
	err = app.tmpl.ExecuteTemplate(pilotBuf, "pilot", td)
	if err == nil {
		e := &sse.Message{}
		e.AppendData(pilotBuf.Bytes())
		app.sseHandler.Publish(e)
	}
}
