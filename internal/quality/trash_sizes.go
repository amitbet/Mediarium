package quality

import (
	"encoding/json"
	"fmt"
	"sort"
)

// TRaSH Guides publish the quality sizes they recommend for Radarr and Sonarr
// (docs/json/<app>/quality-size/*.json): a smallest, a preferred and a largest
// size per quality, in megabytes for each minute. Mediarium's size table has a
// smallest and a largest per quality in the same unit, so those two carry over.
// The preferred size has no counterpart here and is left out.

// trashSizeMax is where Radarr's and Sonarr's sliders end: a largest size at or
// above it means "no upper limit" (TRaSH writes 2000 for movies and 1000 for
// series, with the preferred size one or five below it).
const trashSizeMax = 1000

// trashSizeNames maps the quality names TRaSH uses (Radarr's and Sonarr's) to
// Mediarium's. A second name for the same quality (WEBRip next to WEBDL) is
// only used when the first one is not in the file.
var trashSizeNames = []struct {
	name    string
	tier    Tier
	primary bool
}{
	{"SDTV", TierSDTV, true},
	{"DVD", TierDVD, true},
	{"DVD-R", TierDVD, false},
	{"WEBDL-480p", TierWebDL480p, true},
	{"WEBRip-480p", TierWebDL480p, false},
	{"HDTV-720p", TierHDTV720p, true},
	{"WEBDL-720p", TierWebDL720p, true},
	{"WEBRip-720p", TierWebDL720p, false},
	{"Bluray-720p", TierBluray720p, true},
	{"HDTV-1080p", TierHDTV1080p, true},
	{"WEBDL-1080p", TierWebDL1080p, true},
	{"WEBRip-1080p", TierWebDL1080p, false},
	{"Bluray-1080p", TierBluray1080p, true},
	{"Remux-1080p", TierRemux1080p, true},
	{"Bluray-1080p Remux", TierRemux1080p, true},
	{"HDTV-2160p", TierHDTV2160p, true},
	{"WEBDL-2160p", TierWebDL2160p, true},
	{"WEBRip-2160p", TierWebDL2160p, false},
	{"Bluray-2160p", TierBluray2160p, true},
	{"Remux-2160p", TierRemux2160p, true},
	{"Bluray-2160p Remux", TierRemux2160p, true},
}

// TrashSizes is one TRaSH quality-size file read into Mediarium's terms.
type TrashSizes struct {
	// Limits holds the qualities the file gives a size for.
	Limits map[Tier]SizeLimit
	// Skipped names the qualities in the file that Mediarium has no row for
	// (for example Bluray-576p), in the file's order.
	Skipped []string
}

// ParseTrashSizes reads a TRaSH quality-size file.
func ParseTrashSizes(raw []byte) (TrashSizes, error) {
	var file struct {
		Qualities []struct {
			Quality string  `json:"quality"`
			Min     float64 `json:"min"`
			Max     float64 `json:"max"`
		} `json:"qualities"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return TrashSizes{}, fmt.Errorf("that isn't a TRaSH size list: %w", err)
	}
	if len(file.Qualities) == 0 {
		return TrashSizes{}, fmt.Errorf("that TRaSH size list has no qualities")
	}
	byName := map[string]SizeLimit{}
	order := []string{}
	for _, q := range file.Qualities {
		if q.Min < 0 || q.Max < 0 {
			return TrashSizes{}, fmt.Errorf("the TRaSH size for %s is below 0", q.Quality)
		}
		max := q.Max
		if max >= trashSizeMax {
			max = 0
		}
		if max != 0 && max < q.Min {
			max = q.Min
		}
		byName[q.Quality] = SizeLimit{Min: q.Min, Max: max}
		order = append(order, q.Quality)
	}
	out := TrashSizes{Limits: map[Tier]SizeLimit{}}
	used := map[string]bool{}
	for _, primary := range []bool{true, false} {
		for _, m := range trashSizeNames {
			if m.primary != primary {
				continue
			}
			l, ok := byName[m.name]
			if !ok {
				continue
			}
			used[m.name] = true
			if _, have := out.Limits[m.tier]; !have {
				out.Limits[m.tier] = l
			}
		}
	}
	for _, name := range order {
		if !used[name] {
			out.Skipped = append(out.Skipped, name)
		}
	}
	if len(out.Limits) == 0 {
		return TrashSizes{}, fmt.Errorf("that TRaSH size list has none of the qualities Mediarium knows")
	}
	sort.Strings(out.Skipped)
	return out, nil
}
