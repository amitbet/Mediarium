package library

import "sort"

// How a file's episodes were found.
type placedHow int

const (
	placedAsNamed  placedHow = iota // the show lists the season and episode the name carries
	placedRemapped                  // by the absolute number in the name, or TheXEM's scene numbering
	placedBySeason                  // by TheTVDB's season numbers (Sonarr's), converted to the show's own
	placedByTitle                   // by the episode title in the file name, the last resort
)

// ImportMaps is what is known about a show's other numberings. Either is nil
// when nothing is known.
type ImportMaps struct {
	// Scene says which episode of the show a scene season and episode (TheXEM)
	// stands for.
	Scene func(season, episode int) (int, int, bool)
	// TVDB says which episode of the show a TheTVDB season and episode (the
	// numbers Sonarr names files by) stands for, from the anime lists.
	TVDB func(season, episode int) (int, int, bool)
	// Custom says which episode of the show a season and episode in the
	// person's own numbering for it stands for. It applies to every kind of
	// show and comes before every other rule.
	Custom func(season, episode int) (int, int, bool)
}

// seasonRun is the episode numbers of one season of the show when they are
// one unbroken run.
type seasonRun struct{ first, count int }

// importRules is everything resolveImportFile needs to know about one show.
type importRules struct {
	has   func(season, episode int) bool
	order []Episode
	anime bool
	maps  ImportMaps

	runs     map[int]seasonRun
	inferred map[[2]int][2]int // what the files themselves say, see inferFromFiles
	claimed  map[[2]int]bool   // episodes that a file names for certain

	// The title rule: normalised title -> the episodes that carry it, the
	// episodes that other rules fill, how many files claim an episode by its
	// title, and the seasons where the two season-counting rules disagree
	// with the titles in the file names (see calibrate).
	titles     map[string][][2]int
	taken      map[[2]int]bool
	titleCount map[[2]int]int
	offsetBad  map[int]bool
	inferBad   map[int]bool
}

// newImportRules reads the show's episode list once. files are all the files
// of the show that are placed together (nil when only one is known).
func newImportRules(episodes []Episode, anime bool, maps ImportMaps, files []ImportFile) *importRules {
	listed := make(map[[2]int]bool, len(episodes))
	bySeason := map[int][]int{}
	var r0 map[string][][2]int
	for _, e := range episodes {
		listed[[2]int{e.Season, e.Episode}] = true
		if anime && e.Season > 0 {
			if key := titleKey(e.Title); key != "" {
				if r0 == nil {
					r0 = map[string][][2]int{}
				}
				r0[key] = append(r0[key], [2]int{e.Season, e.Episode})
			}
		}
		if e.Season > 0 && e.Episode > 0 {
			bySeason[e.Season] = append(bySeason[e.Season], e.Episode)
		}
	}
	r := &importRules{
		has:   func(season, episode int) bool { return listed[[2]int{season, episode}] },
		order: AbsoluteOrder(episodes),
		anime: anime,
		maps:  maps,
		runs:  map[int]seasonRun{},

		titles: r0,
	}
	for season, nums := range bySeason {
		sort.Ints(nums)
		unbroken := true
		for i := 1; i < len(nums); i++ {
			if nums[i] != nums[i-1]+1 {
				unbroken = false
				break
			}
		}
		if unbroken {
			r.runs[season] = seasonRun{first: nums[0], count: len(nums)}
		}
	}
	if anime && len(files) > 0 {
		r.inferred = r.inferFromFiles(files)
		// An episode that a file names for certain is never taken by a file
		// that only counts by season.
		r.claimed = map[[2]int]bool{}
		for _, f := range files {
			if t, ok, _ := r.resolveNamed(f); ok {
				for _, n := range t.episodes {
					r.claimed[[2]int{t.season, n}] = true
				}
			}
		}
		r.calibrate(files)
	}
	return r
}

// resolve says which episodes of the show a file is.
func (r *importRules) resolve(f ImportFile) (importTarget, bool, placedHow) {
	if t, ok, how := r.resolveStrong(f); ok {
		return t, true, how
	}
	if t, ok := r.resolveByTitle(f); ok {
		return t, true, placedByTitle
	}
	return importTarget{}, false, placedAsNamed
}

// resolveStrong is every rule except the title one.
func (r *importRules) resolveStrong(f ImportFile) (importTarget, bool, placedHow) {
	if t, ok := r.viaMap(f, r.maps.Custom); ok {
		return t, true, placedRemapped
	}
	if t, ok, how := r.resolveNamed(f); ok {
		return t, true, how
	}
	if !r.anime || f.Season <= 0 || len(f.Episodes) == 0 {
		return importTarget{}, false, placedAsNamed
	}
	t, ok := r.resolveBySeason(f)
	if ok {
		for _, n := range t.episodes {
			if r.claimed[[2]int{t.season, n}] {
				return importTarget{}, false, placedAsNamed
			}
		}
	}
	return t, ok, placedBySeason
}

// resolveNamed is the rules that need nothing but the file's own name: as
// named when the show lists it, and for an anime by its absolute number or
// TheXEM's scene numbering.
func (r *importRules) resolveNamed(f ImportFile) (importTarget, bool, placedHow) {
	for _, n := range f.Episodes {
		if r.has(f.Season, n) {
			return importTarget{f.Season, f.Episodes}, true, placedAsNamed
		}
	}
	if !r.anime || f.Season < 0 || (f.Season == 0 && len(f.Episodes) > 0) {
		return importTarget{}, false, placedAsNamed
	}
	if len(f.Absolute) > 0 {
		t := importTarget{}
		for _, n := range f.Absolute {
			if n < 1 || n > len(r.order) {
				t = importTarget{}
				break
			}
			e := r.order[n-1]
			if t.season != 0 && e.Season != t.season {
				t = importTarget{}
				break
			}
			t.season = e.Season
			t.episodes = append(t.episodes, e.Episode)
		}
		if t.season > 0 && len(t.episodes) > 0 {
			return t, true, placedRemapped
		}
	}
	if t, ok := r.viaMap(f, r.maps.Scene); ok {
		return t, true, placedRemapped
	}
	return importTarget{}, false, placedAsNamed
}

// resolveBySeason places a file named by a season and episode that the show
// does not list, counting the way TheTVDB does (Sonarr's names). Three rules,
// in this order (the last two stand down for a season whose file names carry
// episode titles that say they count differently, see calibrate): the anime lists' TheTVDB seasons; a season of
// the show whose numbers are one run that does not start at 1 (the movie
// database numbers One Piece's season 22 from 1089 to 1155, where TheTVDB
// counts 1 to 67); and the file set itself, when its seasons add up to the
// whole show.
func (r *importRules) resolveBySeason(f ImportFile) (importTarget, bool) {
	if t, ok := r.viaMap(f, r.maps.TVDB); ok {
		return t, true
	}
	if !r.offsetBad[f.Season] {
		if t, ok := r.offsetTarget(f); ok {
			return t, true
		}
	}
	if !r.inferBad[f.Season] {
		if t, ok := r.inferTarget(f); ok {
			return t, true
		}
	}
	return importTarget{}, false
}

// offsetTarget is the season offset rule.
func (r *importRules) offsetTarget(f ImportFile) (importTarget, bool) {
	run, ok := r.runs[f.Season]
	if !ok || run.first <= 1 {
		return importTarget{}, false
	}
	t := importTarget{season: f.Season}
	for _, n := range f.Episodes {
		if n < 1 || n > run.count {
			return importTarget{}, false
		}
		t.episodes = append(t.episodes, run.first+n-1)
	}
	return t, len(t.episodes) > 0
}

// inferTarget is the rule that reads the file set, see inferFromFiles.
func (r *importRules) inferTarget(f ImportFile) (importTarget, bool) {
	if len(r.inferred) == 0 {
		return importTarget{}, false
	}
	t := importTarget{}
	for _, n := range f.Episodes {
		to, ok := r.inferred[[2]int{f.Season, n}]
		if !ok || (t.season != 0 && to[0] != t.season) {
			return importTarget{}, false
		}
		t.season = to[0]
		t.episodes = append(t.episodes, to[1])
	}
	return t, t.season > 0 && len(t.episodes) > 0
}

// viaMap converts the file's season and episodes with another numbering. All
// its episodes must land in one season of the show, on episodes it lists.
func (r *importRules) viaMap(f ImportFile, m func(season, episode int) (int, int, bool)) (importTarget, bool) {
	if m == nil || f.Season <= 0 || len(f.Episodes) == 0 {
		return importTarget{}, false
	}
	t := importTarget{}
	for _, n := range f.Episodes {
		s, e, ok := m(f.Season, n)
		if !ok || !r.has(s, e) || (t.season != 0 && s != t.season) {
			return importTarget{}, false
		}
		t.season = s
		t.episodes = append(t.episodes, e)
	}
	return t, t.season > 0 && len(t.episodes) > 0
}

// inferFromFiles reads the way the files count their seasons when nothing
// else says. It answers only when the files leave no doubt: they hold seasons
// 1 to m, every season is one unbroken run from episode 1, and together they
// are exactly as many episodes as the show has. Then each season's files
// cover the next stretch of the show's episodes, in order (Jujutsu Kaisen's
// 24, 23 and 12 episodes make up the 59 of the movie database's one season).
func (r *importRules) inferFromFiles(files []ImportFile) map[[2]int][2]int {
	have := map[int]map[int]bool{}
	for _, f := range files {
		if f.Season <= 0 || len(f.Episodes) == 0 {
			continue
		}
		if have[f.Season] == nil {
			have[f.Season] = map[int]bool{}
		}
		for _, n := range f.Episodes {
			if n < 1 {
				return nil
			}
			have[f.Season][n] = true
		}
	}
	if len(have) == 0 || len(r.order) == 0 {
		return nil
	}
	total := 0
	for s := 1; s <= len(have); s++ {
		eps := have[s]
		if len(eps) == 0 {
			return nil // the seasons don't run 1 to m
		}
		for n := 1; n <= len(eps); n++ {
			if !eps[n] {
				return nil // a gap: the next season's start would be a guess
			}
		}
		total += len(eps)
	}
	if total != len(r.order) {
		return nil
	}
	out := make(map[[2]int][2]int, total)
	at := 0
	for s := 1; s <= len(have); s++ {
		for n := 1; n <= len(have[s]); n++ {
			e := r.order[at]
			out[[2]int{s, n}] = [2]int{e.Season, e.Episode}
			at++
		}
	}
	return out
}
