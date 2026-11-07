package api

import (
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
)

// indexerFailing reports whether searches on this indexer are failing right
// now: it is paused or has failed since it last answered, or, with no search
// on record, its last test failed.
func indexerFailing(inst indexers.Instance, now time.Time) bool {
	if h, ok := inst.Tracker.Health(inst.ID); ok {
		return h.Failures > 0 || h.Paused(now)
	}
	return inst.LastTestError != ""
}

// indexerSetupHealth adds the Health line about the indexers as a whole, when
// there is one: none added, none turned on, none that can be searched, or none
// that works. It returns the id of the line it added ("" when searching can
// work). Indexers with a key that can't be read are left to their own lines.
// An indexer that exists but fails is never "not set up".
func indexerSetupHealth(instances []indexers.Instance, torrentsOn bool, now time.Time, addHealth func(id, level, title, impact, label, path string)) string {
	var enabled, searchable, failing int
	for _, inst := range instances {
		if inst.SecretError != "" || !inst.Enabled {
			continue
		}
		enabled++
		if inst.Protocol == indexers.ProtocolTorrent && !torrentsOn {
			continue // never searched while torrents are off
		}
		searchable++
		if indexerFailing(inst, now) {
			failing++
		}
	}
	switch {
	case len(instances) == 0:
		addHealth("no-indexers", "error", "No indexer is set up",
			"Without one, searches and automatic downloads find nothing.",
			"Add an indexer", "/settings/indexers")
		return "no-indexers"
	case enabled == 0:
		addHealth("indexers-off", "error", "All your indexers are turned off",
			"Searches and automatic downloads find nothing until one is turned on.",
			"Open indexers", "/settings/indexers")
		return "indexers-off"
	case searchable == 0:
		addHealth("indexers-torrents-off", "error", "Your indexers are all for torrents, and torrents are off",
			"Torrent indexers are not searched while torrents are switched off, so searches and automatic downloads find nothing. Switch torrents on, or add a Usenet indexer.",
			"Open torrent settings", "/settings/torrents")
		return "indexers-torrents-off"
	case failing == searchable:
		addHealth("indexers-failing", "error", "None of your indexers works right now",
			"Every indexer that is turned on failed its last search or test, so searches and automatic downloads find nothing for now. Open an indexer to see why.",
			"Check indexers", "/settings/indexers")
		return "indexers-failing"
	}
	return ""
}
