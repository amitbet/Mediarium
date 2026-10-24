package mediainfo

import "strings"

// Summary is a file's details as short lists for filtering: every value
// appears once, and the lists are comma separated ("en,de").
type Summary struct {
	VCodec  string
	HDR     string
	Res     string
	ACodecs string
	ALangs  string
	SLangs  string
}

// Summary boils the details down for the Library's filters. "Atmos" is added to
// the audio codecs when any track is marked as Atmos.
func (i Info) Summary() Summary {
	var s Summary
	if v := i.Video; v != nil {
		s.VCodec, s.HDR, s.Res = v.Codec, v.HDR, v.Resolution()
	}
	var codecs, alangs, slangs []string
	for _, a := range i.Audio {
		codecs = appendOnce(codecs, a.Codec)
		if a.Atmos {
			codecs = appendOnce(codecs, "Atmos")
		}
		alangs = appendOnce(alangs, a.Lang)
	}
	for _, t := range i.Subtitles {
		slangs = appendOnce(slangs, t.Lang)
	}
	s.ACodecs, s.ALangs, s.SLangs = strings.Join(codecs, ","), strings.Join(alangs, ","), strings.Join(slangs, ",")
	return s
}

// Split turns one of the comma separated lists back into its values.
func Split(list string) []string {
	if list == "" {
		return nil
	}
	return strings.Split(list, ",")
}

func appendOnce(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, have := range list {
		if have == v {
			return list
		}
	}
	return append(list, v)
}
