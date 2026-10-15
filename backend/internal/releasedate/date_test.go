package releasedate

import "testing"

func TestDatePrecisionAndValidation(t *testing.T) {
	for _, test := range []struct {
		value, date, precision string
		year                   int
	}{
		{"1989", "1989", "year", 1989}, {"1989-03", "1989-03", "month", 1989}, {"1989-03-21", "1989-03-21", "day", 1989},
		{"1989-03-21T00:00:00Z", "1989-03-21", "day", 1989}, {"1989-02-30", "", "", 0}, {"0000", "", "", 0}, {"198x", "", "", 0}, {"1989-99", "", "", 0},
	} {
		date, precision, year := Parse(test.value)
		if date != test.date || precision != test.precision || year != test.year {
			t.Fatalf("%q: %q %q %d", test.value, date, precision, year)
		}
	}
}
