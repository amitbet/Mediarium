package indexers

import "context"

// servesCategories reports whether the site can answer a search in cats: one
// of its categories is one of them, or a sub-category (or the parent) of one.
// A site that lists no categories at all is taken to serve everything, since a
// search of it asks for none.
func (d *Definition) servesCategories(cats []int) bool {
	if len(cats) == 0 {
		return true
	}
	want := expandCategories(cats)
	for _, c := range cats {
		want[c/1000*1000] = true // a site with only "Audio" may well hold audiobooks
	}
	listed := false
	for _, m := range d.mappings() {
		if m.newznab == 0 {
			continue
		}
		listed = true
		if want[m.newznab] {
			return true
		}
	}
	return !listed
}

// ServesCategories reports whether a definition-based site lists a category
// in cats, without asking the site anything: the categories are in its
// definition. When the definition can't be read it says true, so the search
// itself reports what is wrong.
func (m *CardigannManager) ServesCategories(ctx context.Context, inst Instance, cats []int) bool {
	if m == nil {
		return true
	}
	sess, err := m.session(ctx, inst)
	if err != nil || sess == nil || sess.def == nil {
		return true
	}
	return sess.def.servesCategories(cats)
}

// SplitByCategories sorts instances into the ones that can answer a search in
// cats and the ones that can't, so a search for books does not ask a movie
// site. Definition-based sites are judged by the categories their definition
// lists. Newznab and Torznab indexers are always asked: Mediarium does not
// keep their capabilities, and an indexer that has none of the category simply
// answers with nothing.
func SplitByCategories(ctx context.Context, instances []Instance, cats []int) (capable, skipped []Instance) {
	for _, inst := range instances {
		if inst.IsCardigann() && !inst.Cardigann.ServesCategories(ctx, inst, cats) {
			skipped = append(skipped, inst)
			continue
		}
		capable = append(capable, inst)
	}
	return capable, skipped
}
