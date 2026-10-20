package cli

import (
	"encoding/json"
	"testing"
)

func TestPeriodDateDecodesEveryShape(t *testing.T) {
	cases := []struct {
		name, in string
		want     periodDate
	}{
		{"object with month", `{"year":2024,"month":11}`, "Nov 2024"},
		{"object, year only", `{"year":2018}`, "2018"},
		{"object without a year", `{}`, ""},
		{"legacy string", `"Mar 2021"`, "Mar 2021"},
		{"bare year", `2019`, "2019"},
		{"null", `null`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got periodDate
			if err := json.Unmarshal([]byte(c.in), &got); err != nil {
				t.Fatalf("decode %s: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("decode %s = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
