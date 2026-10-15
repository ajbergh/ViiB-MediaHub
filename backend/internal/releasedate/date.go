// Package releasedate separates recording first-release dates from edition dates.
package releasedate

import (
	"strconv"
	"strings"
	"time"
)

func Parse(value string) (date, precision string, year int) {
	value = strings.TrimSpace(value)
	if len(value) > 10 && value[10] == 'T' {
		value = value[:10]
	}
	layout := ""
	switch len(value) {
	case 4:
		layout, precision = "2006", "year"
	case 7:
		layout, precision = "2006-01", "month"
	case 10:
		layout, precision = "2006-01-02", "day"
	default:
		return "", "", 0
	}
	parsed, err := time.Parse(layout, value)
	if err != nil || parsed.Year() < 1000 || parsed.Year() > time.Now().Year()+1 {
		return "", "", 0
	}
	return value, precision, parsed.Year()
}

func Year(value string) int { _, _, year := Parse(value); return year }
func YearDate(year int) string {
	if year < 1000 || year > time.Now().Year()+1 {
		return ""
	}
	return strconv.Itoa(year)
}
