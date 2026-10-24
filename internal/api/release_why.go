package api

import "fmt"

// The interactive search list can say why a release scored what it did and why
// an automatic search would skip it. Everything here is worked out when the
// list is built; nothing is stored.

// releaseReason is one thing that makes an automatic search leave a release
// out or rank it lower, in plain words. Kind groups them for the screen:
// profile, quality, language, upgrade, size, age, release, blocklist, seeders,
// and notBest, which is no reason to skip a release but says why another one
// was taken instead (see not_best.go).
type releaseReason struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func reasonTexts(rs []releaseReason) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Text)
	}
	return out
}

// addReasonList adds reasons that also count as rejections: the best match
// never takes a release that has one.
func (p *searchResultPayload) addReasonList(rs []releaseReason) {
	for _, r := range rs {
		p.Rejections = append(p.Rejections, r.Text)
		p.Reasons = append(p.Reasons, r)
	}
}

// addReasons is addReasonList for plain texts of one kind.
func (p *searchResultPayload) addReasons(kind string, texts []string) {
	for _, t := range texts {
		p.addReasonList([]releaseReason{{Kind: kind, Text: t}})
	}
}

// addStateReasons adds the reasons that come from flags already on the
// payload (blocklisted, too few seeders, another language). They are only
// listed: the flags themselves already keep the best match away from them.
func (p *searchResultPayload) addStateReasons() {
	if p.Blocklisted {
		p.Reasons = append(p.Reasons, releaseReason{"blocklist", "This release failed before and is on the blocklist, so automatic searches leave it out."})
	}
	if p.BelowMinSeeders {
		p.Reasons = append(p.Reasons, releaseReason{"seeders", fmt.Sprintf("Its seeder count (%d) is below the minimum of %d set for its indexer, so automatic searches leave it out.", p.Seeders, p.MinSeeders)})
	}
	if p.LanguageFit == "other" {
		p.Reasons = append(p.Reasons, releaseReason{"language", "It is in another language than the one you prefer, so automatic searches leave it out."})
	}
}
