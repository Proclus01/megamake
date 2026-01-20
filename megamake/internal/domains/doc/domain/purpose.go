package domain

import (
	"regexp"
	"sort"
	"strings"
)

type PurposeInput struct {
	ReadmeText string
	Languages  []string
	SampleText []string // a handful of file contents (already truncated)
}

func GuessPurpose(in PurposeInput) string {
	var lines []string

	readme := strings.TrimSpace(in.ReadmeText)
	if readme != "" {
		head := firstLines(readme, 8)
		lines = append(lines, "README summary (first lines):")
		lines = append(lines, head)
	}

	if len(in.Languages) > 0 {
		langs := append([]string(nil), in.Languages...)
		sort.Strings(langs)
		lines = append(lines, "")
		lines = append(lines, "Detected languages: "+strings.Join(langs, ", "))
	}

	// Capability hints (regex patterns; intentionally crude).
	hints := map[string]string{
		"web":   `express|fastapi|flask|spring|ktor|actix|router\.|http\.|net/http|urlsession|axios|fetch|GetMapping`,
		"db":    `sqlalchemy|psycopg2|gorm|database/sql|entitymanager|jpa|mongoose|redis`,
		"cli":   `argparse|click|cobra|commander|swift-argument-parser`,
		"ml":    `torch|tensorflow|keras|sklearn`,
		"queue": `kafka|rabbitmq|pubsub|sqs`,
		"cloud": `aws|gcp|azure|s3|bigquery|pubsub|blob|cosmos`,
		"test":  `pytest|jest|vitest|xctest|go test|cargo test`,
	}

	counts := map[string]int{}
	for _, s := range in.SampleText {
		lower := strings.ToLower(s)
		for k, pat := range hints {
			if regexp.MustCompile(pat).FindStringIndex(lower) != nil {
				counts[k]++
			}
		}
	}

	if len(counts) > 0 {
		type kv struct {
			k string
			v int
		}
		var items []kv
		for k, v := range counts {
			items = append(items, kv{k: k, v: v})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].v == items[j].v {
				return items[i].k < items[j].k
			}
			return items[i].v > items[j].v
		})
		var keys []string
		for _, it := range items {
			keys = append(keys, it.k)
		}
		lines = append(lines, "")
		lines = append(lines, "Capability hints: "+strings.Join(keys, ", "))
	}

	if len(lines) == 0 {
		if len(in.Languages) > 0 {
			return "No README and limited hints; likely a library or service in " + in.Languages[0]
		}
		return "No README and limited hints; likely a library or service."
	}

	return strings.Join(lines, "\n")
}

func firstLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	parts := strings.Split(s, "\n")
	if len(parts) <= n {
		return strings.Join(parts, "\n")
	}
	return strings.Join(parts[:n], "\n")
}
