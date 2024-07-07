package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type Response struct {
	Departures []Departure `json:"departures"`
}

type Departure struct {
	Destination string     `json:"destination"`
	Direction   string     `json:"direction"`
	Scheduled   CustomTime `json:"scheduled"`
	Expected    CustomTime `json:"expected"`
	Line        Line       `json:"line"`
}

type Line struct {
	ID          int    `json:"id"`
	Designation string `json:"designation"`
}

type CustomTime struct {
	time.Time
}

func (ct CustomTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(ct.Format("2006-01-02T15:04:05"))
}

func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = s[1 : len(s)-1]
	t, err := time.Parse("2006-01-02T15:04:05", s)
	if err != nil {
		return err
	}
	ct.Time = t
	return nil
}

const baseURL = "https://transport.integration.sl.se/v1/sites/%s/departures"

func main() {
	http.HandleFunc("/departures", handleDepartures)
	http.HandleFunc("/departures/json", handleDeparturesJSON)
	fmt.Println("Server is running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleDepartures(w http.ResponseWriter, r *http.Request) {
	departures, siteID, err := getDepartures(r)
	if err != nil {
		http.Error(w, "Error fetching departure data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	prettyPrintDepartures(w, departures, siteID)
}

func handleDeparturesJSON(w http.ResponseWriter, r *http.Request) {
	departures, _, err := getDepartures(r)
	if err != nil {
		http.Error(w, "Error fetching departure data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(departures) == 0 {
		departures = []Departure{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(departures)
}

func getDepartures(r *http.Request) ([]Departure, string, error) {
	siteID := r.URL.Query().Get("siteId")
	if siteID == "" {
		return nil, "", fmt.Errorf("siteId query parameter is required")
	}

	response, err := fetchDepartures(siteID)
	if err != nil {
		return nil, siteID, err
	}

	lineID := r.URL.Query().Get("lineId")
	direction := r.URL.Query().Get("direction")

	filteredDepartures := filterDepartures(response.Departures, lineID, direction)

	sort.Slice(filteredDepartures, func(i, j int) bool {
		return filteredDepartures[i].Expected.Before(filteredDepartures[j].Expected.Time)
	})

	return filteredDepartures, siteID, nil
}

func fetchDepartures(siteID string) (Response, error) {
	var response Response

	url := fmt.Sprintf(baseURL, siteID)
	resp, err := http.Get(url)
	if err != nil {
		return response, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("error reading response body: %v", err)
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return response, fmt.Errorf("error parsing JSON: %v", err)
	}

	return response, nil
}

func filterDepartures(departures []Departure, lineID, direction string) []Departure {
	var filtered []Departure

	for _, d := range departures {
		if lineID != "" {
			id, err := strconv.Atoi(lineID)
			if err != nil || d.Line.ID != id {
				continue
			}
		}

		if direction != "" && d.Direction != direction {
			continue
		}

		filtered = append(filtered, d)
	}

	return filtered
}

func prettyPrintDepartures(w http.ResponseWriter, departures []Departure, siteID string) {
	if len(departures) == 0 {
		fmt.Fprintf(w, "No departures found matching the criteria for site ID: %s\n", siteID)
		return
	}

	fmt.Fprintf(w, "Upcoming Departures for site ID %s (sorted by expected departure time):\n", siteID)
	fmt.Fprintln(w, "--------------------")

	for _, d := range departures {
		scheduledTime := d.Scheduled.Format("15:04")
		expectedTime := d.Expected.Format("15:04")

		fmt.Fprintf(w, "Line %s (ID: %d) to %s\n", d.Line.Designation, d.Line.ID, d.Destination)
		fmt.Fprintf(w, "  Direction: %s\n", d.Direction)
		fmt.Fprintf(w, "  Scheduled: %s\n", scheduledTime)
		fmt.Fprintf(w, "  Expected:  %s\n", expectedTime)

		if scheduledTime != expectedTime {
			delay := d.Expected.Sub(d.Scheduled.Time)
			fmt.Fprintf(w, "  Delay:     %d minutes\n", int(delay.Minutes()))
		}

		fmt.Fprintln(w, "--------------------")
	}
}
