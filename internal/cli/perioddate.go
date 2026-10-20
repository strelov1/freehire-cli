package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// periodDate is an employment boundary as the CLI prints and sends it: "Mar 2021", or
// "2021" for a year-only date. The API serves the boundary as {"year":2021,"month":3}
// and accepts that same free text back on a write, so the CLI keeps one string form for
// both directions.
//
// Decoding accepts the object, a plain string (the shape the API served before it
// switched) and a bare year, so a change in how the server spells a date cannot take
// the write path down with the read path again (freehire-cli#41: decoding into a
// string blocked every employment update and atom removal).
type periodDate string

var periodMonths = [...]string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func (p *periodDate) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*p = ""
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*p = periodDate(s)
		return nil
	case '{':
		var d struct {
			Year  int `json:"year"`
			Month int `json:"month"`
		}
		if err := json.Unmarshal(b, &d); err != nil {
			return err
		}
		*p = formatPeriod(d.Year, d.Month)
		return nil
	default:
		var year int
		if err := json.Unmarshal(b, &year); err != nil {
			return err
		}
		*p = formatPeriod(year, 0)
		return nil
	}
}

// formatPeriod renders a date the way the API's own formatter does, which is also a
// form its parser reads back. A missing year is no date at all.
func formatPeriod(year, month int) periodDate {
	if year <= 0 {
		return ""
	}
	if month < 1 || month > 12 {
		return periodDate(strconv.Itoa(year))
	}
	return periodDate(periodMonths[month] + " " + strconv.Itoa(year))
}
