package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Source groups an agent can filter search by, named for what it is looking
// for rather than for where each corpus source was scraped from. A group name
// ending in "-" matches every source with that prefix (all the wikis, all the
// forums), so a source added upstream joins its group without a code change.
var sourceGroups = map[string][]string{
	"api":       {"gscode-api"},                                                 // GSC/CSC function reference
	"scripts":   {"source-scripts", "source-dump"},                              // Treyarch's shipped scripts and data
	"docs":      {"docs-bo3", "defs-bo3", "tools-dtzxporter", "personal-notes"}, // official docs, asset/entity schemas, tool docs
	"wiki":      {"wiki-"},
	"forums":    {"forums-"},
	"discord":   {"discord-bo3modtools"},
	"video":     {"video-youtube"},
	"workspace": {"source-workspace"}, // files of a mod-tools install
}

// SourceGroups lists the group names, for help texts.
func SourceGroups() []string {
	out := make([]string, 0, len(sourceGroups))
	for g := range sourceGroups {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// ResolveSources turns group names and exact source names into the exact
// source names they cover in this database. An unknown name is an error that
// lists what exists, so a typo can't pass for "no results".
func (s *Store) ResolveSources(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	all, err := s.sources(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(src string) {
		if !seen[src] {
			seen[src] = true
			out = append(out, src)
		}
	}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" {
			continue
		}
		matched := false
		for _, pat := range sourceGroups[n] {
			for _, src := range all {
				if src == pat || (strings.HasSuffix(pat, "-") && strings.HasPrefix(src, pat)) {
					add(src)
					matched = true
				}
			}
		}
		for _, src := range all {
			if src == n {
				add(src)
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("unknown source %q: use a group (%s) or a source name (%s)", n, strings.Join(SourceGroups(), ", "), strings.Join(all, ", "))
		}
	}
	sort.Strings(out)
	return out, nil
}

// sources lists the database's distinct sources, once per Store.
func (s *Store) sources(ctx context.Context) ([]string, error) {
	s.srcOnce.Do(func() {
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT source FROM documents ORDER BY source`)
		if err != nil {
			s.srcErr = err
			return
		}
		defer rows.Close()
		for rows.Next() {
			var src string
			if err := rows.Scan(&src); err != nil {
				s.srcErr = err
				return
			}
			s.srcList = append(s.srcList, src)
		}
		s.srcErr = rows.Err()
	})
	return s.srcList, s.srcErr
}

func sourceSet(sources []string) map[string]bool {
	if len(sources) == 0 {
		return nil
	}
	set := make(map[string]bool, len(sources))
	for _, src := range sources {
		set[src] = true
	}
	return set
}

// sourceOf is a doc_id's source: doc_ids are "<source>::<local id>"
// (docs/data-model.md; every document of the v2.2 corpus follows it).
func sourceOf(docID string) string {
	src, _, _ := strings.Cut(docID, "::")
	return src
}
