package pilot

import "strings"

func splitPath(path string) []string {
	p := path
	if p == "" {
		return []string{""}
	}

	p = strings.Trim(p, " ")

	if idx := strings.IndexByte(p, '?'); idx >= 0 {
		p = p[:idx]
	}
	if idx := strings.IndexByte(p, '#'); idx >= 0 {
		p = p[:idx]
	}

	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if p == "/" {
		return []string{""}
	}
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}

	segs := strings.Split(p, "/")
	if len(segs) > 0 && segs[0] == "" {
		segs = segs[1:]
	}

	return segs
}
