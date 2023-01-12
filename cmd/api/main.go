package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/r3labs/sse/v2"
	"io"
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
	sseServer *sse.Server
	cfg       config
}

func main() {
	var cfg config

	flag.IntVar(&cfg.serverPort, "port", getEnvInt("PORT", 8080), "API server port")
	flag.IntVar(&cfg.sleepDuration, "sleep", 2000, "Timeout between drone position polls (milliseconds)")
	flag.Float64Var(&cfg.noFlyZoneRadius, "no-fly-zone-radius", 100, "Radius of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginX, "no-fly-zone-origin-x", 250000, "Origin X coordinate of no-fly zone in meters")
	flag.Float64Var(&cfg.noFlyZoneOriginY, "no-fly-zone-origin-y", 250000, "Origin Y coordinate of no-fly zone in meters")

	flag.Parse()

	server := sse.New()
	server.CreateStream("birdnest")

	app := application{
		sseServer: server,
		cfg:       cfg,
	}

	go app.monitor()
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.serverPort), app.routes())
}

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", app.sseServer.ServeHTTP)

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
		Text              string `xml:",chardata"`
		SnapshotTimestamp string `xml:"snapshotTimestamp,attr"`
		Drone             []struct {
			Text         string `xml:",chardata"`
			SerialNumber string `xml:"serialNumber"`
			Model        string `xml:"model"`
			Manufacturer string `xml:"manufacturer"`
			Mac          string `xml:"mac"`
			Ipv4         string `xml:"ipv4"`
			Ipv6         string `xml:"ipv6"`
			Firmware     string `xml:"firmware"`
			PositionY    string `xml:"positionY"`
			PositionX    string `xml:"positionX"`
			Altitude     string `xml:"altitude"`
		} `xml:"drone"`
	} `xml:"capture"`
}

func (app *application) monitor() {
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
			fmt.Println(drone.SerialNumber)
		}
	}
}
