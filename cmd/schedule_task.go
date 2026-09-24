package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
)

// taskDoc is the subset of the Task Scheduler schema Duster writes and reads
// back. Field order follows the schema (CalendarTrigger is a sequence).
type taskDoc struct {
	XMLName      xml.Name      `xml:"http://schemas.microsoft.com/windows/2004/02/mit/task Task"`
	Version      string        `xml:"version,attr"`
	Author       string        `xml:"RegistrationInfo>Author"`
	Description  string        `xml:"RegistrationInfo>Description"`
	Start        string        `xml:"Triggers>CalendarTrigger>StartBoundary"`
	TriggerOn    bool          `xml:"Triggers>CalendarTrigger>Enabled"`
	DaysInterval int           `xml:"Triggers>CalendarTrigger>ScheduleByDay>DaysInterval"`
	Principal    taskPrincipal `xml:"Principals>Principal"`
	Settings     taskSettings  `xml:"Settings"`
	Actions      taskActions   `xml:"Actions"`
}

type taskPrincipal struct {
	ID        string `xml:"id,attr"`
	UserID    string `xml:"UserId"`
	LogonType string `xml:"LogonType"`
	RunLevel  string `xml:"RunLevel"`
}

type taskSettings struct {
	MultipleInstancesPolicy    string `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool   `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool   `xml:"StopIfGoingOnBatteries"`
	StartWhenAvailable         bool   `xml:"StartWhenAvailable"`
	ExecutionTimeLimit         string `xml:"ExecutionTimeLimit"`
	Priority                   int    `xml:"Priority"`
	// Enabled is nil when an exported task leaves out the default (true).
	Enabled *bool `xml:"Enabled"`
}

type taskActions struct {
	Context   string `xml:"Context,attr"`
	Command   string `xml:"Exec>Command"`
	Arguments string `xml:"Exec>Arguments"`
}

// buildTaskXML returns the task definition for schtasks /Create /XML, encoded
// UTF-16LE with a BOM as Task Scheduler expects. The trigger fires daily at
// cfg.At from today; `run` decides whether a clean is due.
func buildTaskXML(cfg scheduleConfig, duw, userSID string, now time.Time) ([]byte, error) {
	enabled := true
	doc := taskDoc{
		Version:      "1.2",
		Author:       "Duster",
		Description:  "Cleans caches on a schedule. Manage it with: du schedule",
		Start:        now.Format("2006-01-02") + "T" + cfg.At + ":00",
		TriggerOn:    true,
		DaysInterval: 1,
		Principal: taskPrincipal{
			ID: "Author", UserID: userSID,
			LogonType: "InteractiveToken", RunLevel: "LeastPrivilege",
		},
		Settings: taskSettings{
			MultipleInstancesPolicy:    "IgnoreNew",
			DisallowStartIfOnBatteries: true,
			StopIfGoingOnBatteries:     true,
			StartWhenAvailable:         true,
			ExecutionTimeLimit:         "PT1H",
			Priority:                   7,
			Enabled:                    &enabled,
		},
		// Quoted like Task Scheduler's own exports, so a path with spaces is one program.
		Actions: taskActions{
			Context:   "Author",
			Command:   `"` + duw + `"`,
			Arguments: strings.Join(cfg.taskArguments(), " "),
		},
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	units := utf16.Encode([]rune(`<?xml version="1.0" encoding="UTF-16"?>` + "\n" + string(body)))
	out := make([]byte, 0, 2+2*len(units))
	out = append(out, 0xFF, 0xFE)
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out, nil
}

// parseTaskXML reads schtasks /Query /XML output, which may be UTF-16LE (with
// or without a BOM) or single-byte, while always declaring UTF-16.
func parseTaskXML(b []byte) (taskDoc, error) {
	b = bytes.TrimPrefix(b, []byte{0xFF, 0xFE})
	text := strings.TrimPrefix(decodeWSLOutput(b), string([]rune{0xFEFF}))
	d := xml.NewDecoder(strings.NewReader(text))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil } // already decoded
	var doc taskDoc
	err := d.Decode(&doc)
	return doc, err
}

// parseTaskNames returns the Duster task names in schtasks /Query /FO CSV /NH
// output. Only the name column is read: names are not translated, the rest is.
func parseTaskNames(csvOut []byte) []string {
	r := csv.NewReader(strings.NewReader(decodeWSLOutput(csvOut)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var names []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) == 0 {
			continue
		}
		name := strings.TrimPrefix(rec[0], `\`)
		if strings.HasPrefix(name, "Duster Scheduled Clean (") && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
