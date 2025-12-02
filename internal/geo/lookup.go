// Package geo provides IP geolocation lookup.
package geo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/miguelemosreverte/vpn/internal/protocol"
)

const (
	// ip-api.com free tier (no API key needed, 45 req/min limit)
	ipAPIURL = "http://ip-api.com/json/%s?fields=status,message,country,city,lat,lon,isp,query"
	// Timeout for geolocation lookup
	lookupTimeout = 5 * time.Second
)

// ipAPIResponse is the response from ip-api.com
type ipAPIResponse struct {
	Status  string  `json:"status"`
	Message string  `json:"message,omitempty"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	ISP     string  `json:"isp"`
	Query   string  `json:"query"` // The IP that was looked up
}

// LookupIP returns geolocation for a specific IP address.
func LookupIP(ip string) (*protocol.GeoLocation, error) {
	client := &http.Client{Timeout: lookupTimeout}

	url := fmt.Sprintf(ipAPIURL, ip)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup IP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ipAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("lookup failed: %s", result.Message)
	}

	return &protocol.GeoLocation{
		Latitude:  result.Lat,
		Longitude: result.Lon,
		City:      result.City,
		Country:   result.Country,
		ISP:       result.ISP,
	}, nil
}

// LookupSelf returns geolocation for this machine's public IP.
// This should be called BEFORE connecting to VPN to get the real location.
func LookupSelf() (*protocol.GeoLocation, string, error) {
	client := &http.Client{Timeout: lookupTimeout}

	// Empty IP means lookup the caller's IP
	url := fmt.Sprintf(ipAPIURL, "")
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to lookup self: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var result ipAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, "", fmt.Errorf("lookup failed: %s", result.Message)
	}

	geo := &protocol.GeoLocation{
		Latitude:  result.Lat,
		Longitude: result.Lon,
		City:      result.City,
		Country:   result.Country,
		ISP:       result.ISP,
	}

	return geo, result.Query, nil
}
