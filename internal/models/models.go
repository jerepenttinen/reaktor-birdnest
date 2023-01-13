package models

import (
	"encoding/xml"
	"time"
)

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
		Drone             []struct {
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
		} `xml:"drone"`
	} `xml:"capture"`
}

type Pilot struct {
	PilotID     string    `json:"pilotId"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	PhoneNumber string    `json:"phoneNumber"`
	CreatedDt   time.Time `json:"createdDt"`
	Email       string    `json:"email"`
}
