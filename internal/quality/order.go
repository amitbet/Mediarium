package quality

import (
	"encoding/json"
	"fmt"
	"strings"
)

// A profile can carry its own quality order. Each item of the order is one
// rank: either a single quality or a named group of several, and the order is
// best first, like the quality list of Radarr and Sonarr. A profile without an
// order (QualityOrder empty) uses the built-in ladder, exactly as before.

const (
	maxOrderItems   = 40
	maxGroupNameLen = 40
	rankStep        = 100
)

// QualityItem is one rank in a profile's quality order: a single tier, or a
// group of tiers that count as the same quality. Name is only used by groups.
type QualityItem struct {
	Name  string `json:"name,omitempty"`
	Tiers []Tier `json:"tiers"`
}

// IsGroup reports whether the item is a named group rather than one quality.
func (it QualityItem) IsGroup() bool { return it.Name != "" }

func (it QualityItem) has(t Tier) bool {
	for _, x := range it.Tiers {
		if x == t {
			return true
		}
	}
	return false
}

// HasCustomOrder reports whether the profile has its own quality order.
func (p Profile) HasCustomOrder() bool { return len(p.QualityOrder) > 0 }

// ItemOf returns the position (0 = best) of the order item that holds t, or -1
// when the profile has no order or the tier is not in it.
func (p Profile) ItemOf(t Tier) int {
	for i, it := range p.QualityOrder {
		if it.has(t) {
			return i
		}
	}
	return -1
}

// TierRank is the rank of a tier for this profile (higher = better). It is the
// one place that decides which quality beats which: the automatic pick, the
// interactive search, upgrades and the cutoff all go through it. Without a
// custom order it is the built-in ladder position (Rank). With one, a tier
// ranks as its item does, so every tier in a group has the same rank. A tier the
// order doesn't list ranks just above the listed tier that is the next one down
// the built-in ladder (or at the bottom when there is none), so a file better
// than anything the profile allows is not mistaken for a poor one.
func (p Profile) TierRank(t Tier) int {
	if len(p.QualityOrder) == 0 {
		return Rank(t)
	}
	n := len(p.QualityOrder)
	if i := p.ItemOf(t); i >= 0 {
		return (n - i) * rankStep
	}
	best, bestRank := -1, -1
	for i, it := range p.QualityOrder {
		for _, x := range it.Tiers {
			if r := Rank(x); r <= Rank(t) && r > bestRank {
				best, bestRank = i, r
			}
		}
	}
	if best < 0 {
		return Rank(t) // below everything listed: still told apart by the ladder
	}
	// The distance down the ladder keeps unlisted tiers in their usual order.
	return (n-best)*rankStep + 1 + Rank(t) - bestRank
}

// HighestRank is the rank of the best quality this profile allows.
func (p Profile) HighestRank() int {
	top := 0
	for _, t := range p.Allowed {
		if r := p.TierRank(t); r > top {
			top = r
		}
	}
	return top
}

// CutoffItem returns the order item the cutoff sits in, and false when the
// profile has no order.
func (p Profile) CutoffItem() (QualityItem, bool) {
	i := p.ItemOf(p.Cutoff)
	if i < 0 {
		return QualityItem{}, false
	}
	return p.QualityOrder[i], true
}

// CutoffName is how the cutoff is shown: the name of the group it sits in, or
// the quality itself.
func (p Profile) CutoffName() string {
	if it, ok := p.CutoffItem(); ok && it.IsGroup() {
		return it.Name
	}
	return string(p.Cutoff)
}

// DefaultOrder is the built-in order written out as items, best first, for the
// given allowed tiers.
func DefaultOrder(allowed []Tier) []QualityItem {
	has := map[Tier]bool{}
	for _, t := range allowed {
		has[t] = true
	}
	var out []QualityItem
	for i := len(ladder) - 1; i >= 0; i-- {
		if has[ladder[i]] {
			out = append(out, QualityItem{Tiers: []Tier{ladder[i]}})
		}
	}
	return out
}

// cleanOrder checks and tidies a quality order: every tier real and listed
// once, no empty items, group names unique and a name on every group. Within
// an item the tiers are kept best first by the built-in ladder, which is also
// how a tie inside a group is settled.
func cleanOrder(items []QualityItem) ([]QualityItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > maxOrderItems {
		return nil, fmt.Errorf("%w: at most %d qualities or groups in the order", ErrInvalid, maxOrderItems)
	}
	valid := map[Tier]bool{}
	for _, t := range AllTiers() {
		valid[t] = true
	}
	seen := map[Tier]bool{}
	names := map[string]bool{}
	out := make([]QualityItem, 0, len(items))
	for _, it := range items {
		it.Name = strings.TrimSpace(it.Name)
		if len(it.Name) > maxGroupNameLen || strings.ContainsAny(it.Name, "\n\r") {
			return nil, fmt.Errorf("%w: a group name must be under %d characters", ErrInvalid, maxGroupNameLen)
		}
		if len(it.Tiers) == 0 {
			return nil, fmt.Errorf("%w: a group in the order has no qualities", ErrInvalid)
		}
		if it.Name != "" {
			key := strings.ToLower(it.Name)
			if names[key] {
				return nil, fmt.Errorf("%w: two groups are called %q", ErrInvalid, it.Name)
			}
			names[key] = true
		}
		in := map[Tier]bool{}
		for _, t := range it.Tiers {
			if !valid[t] {
				return nil, fmt.Errorf("%w: %q is not a quality tier", ErrInvalid, t)
			}
			if seen[t] {
				return nil, fmt.Errorf("%w: %s is in the order twice", ErrInvalid, t)
			}
			seen[t] = true
			in[t] = true
		}
		sorted := make([]Tier, 0, len(in))
		for i := len(ladder) - 1; i >= 0; i-- {
			if in[ladder[i]] {
				sorted = append(sorted, ladder[i])
			}
		}
		it.Tiers = sorted
		if len(it.Tiers) > 1 && it.Name == "" {
			return nil, fmt.Errorf("%w: a group of qualities needs a name", ErrInvalid)
		}
		out = append(out, it)
	}
	return out, nil
}

// orderTiers is every tier in the order, sorted by the built-in ladder.
func orderTiers(items []QualityItem) []Tier {
	in := map[Tier]bool{}
	for _, it := range items {
		for _, t := range it.Tiers {
			in[t] = true
		}
	}
	var out []Tier
	for _, t := range ladder {
		if in[t] {
			out = append(out, t)
		}
	}
	return out
}

func encodeOrder(items []QualityItem) string {
	if len(items) == 0 {
		return ""
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func decodeOrder(s string) []QualityItem {
	if s == "" {
		return nil
	}
	var items []QualityItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil
	}
	return items
}
