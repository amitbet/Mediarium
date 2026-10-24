package metadata

import (
	"sort"
	"strings"
)

// MovieReleaseDates are the dates a film opens in the cinema and comes out
// digitally, for one country.
type MovieReleaseDates struct {
	// Cinema is the premiere or theatrical date ("YYYY-MM-DD"), Digital the
	// date it can be bought or rented online; empty when TMDB has none.
	Cinema  string `json:"cinema,omitempty"`
	Digital string `json:"digital,omitempty"`
	// Region is the country asked for. CinemaRegion and DigitalRegion name the
	// country a date really came from when it was not that one (only ever the
	// US, since a small country often lists nothing).
	Region        string `json:"region,omitempty"`
	CinemaRegion  string `json:"cinemaRegion,omitempty"`
	DigitalRegion string `json:"digitalRegion,omitempty"`
}

// RegionOfLanguage is the country part of a TMDB language tag ("de-DE" gives
// "DE"), or "US" when the tag has none.
func RegionOfLanguage(tag string) string {
	if _, region, ok := strings.Cut(tag, "-"); ok && len(region) == 2 {
		return strings.ToUpper(region)
	}
	return "US"
}

// TMDB release types: 1 premiere, 2 limited theatrical, 3 theatrical, 4
// digital, 5 physical, 6 TV. Cinema takes the first type a country has.
var (
	cinemaTypes  = []int{3, 2, 1}
	digitalTypes = []int{4}
)

// earliest is the first date of the wanted types (tried in order) that a
// country lists, or "" when it lists none. Dates arrive as
// "2025-03-12T00:00:00.000Z"; only the day is kept.
func (d *MovieDetail) earliest(country string, types []int) string {
	for _, want := range types {
		var dates []string
		for _, c := range d.ReleaseDates.Results {
			if c.Country != country {
				continue
			}
			for _, rd := range c.ReleaseDates {
				if rd.Type == want && len(rd.Date) >= 10 {
					dates = append(dates, rd.Date[:10])
				}
			}
		}
		if len(dates) > 0 {
			sort.Strings(dates)
			return dates[0]
		}
	}
	return ""
}

// ReleaseDatesFor gives the cinema and digital dates in a country. A date the
// country does not list falls back to the US (and is tagged as coming from
// there). There is no other fallback: a date from some random country means
// nothing to the person, so a date neither lists stays empty.
func (d *MovieDetail) ReleaseDatesFor(region string) MovieReleaseDates {
	region = strings.ToUpper(region)
	out := MovieReleaseDates{Region: region}
	pick := func(types []int) (string, string) {
		if got := d.earliest(region, types); got != "" {
			return got, ""
		}
		if got := d.earliest("US", types); got != "" {
			return got, "US"
		}
		return "", ""
	}
	out.Cinema, out.CinemaRegion = pick(cinemaTypes)
	out.Digital, out.DigitalRegion = pick(digitalTypes)
	return out
}
