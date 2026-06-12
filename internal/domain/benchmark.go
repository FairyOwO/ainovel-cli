package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	BenchmarkProfileVersion      = "benchmark_profile.v1"
	maxCompactBenchmarkItems     = 8
	maxCompactBenchmarkSummaries = 12
)

var benchmarkNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Benchmark struct {
	BenchmarkCompact
	CreatedAt string `json:"created_at,omitempty"`
	Source    string `json:"source,omitempty"`
}

type BenchmarkCompact struct {
	Version            string   `json:"version"`
	Name               string   `json:"name"`
	Title              string   `json:"title,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Structure          []string `json:"structure,omitempty"`
	Pacing             []string `json:"pacing,omitempty"`
	Hooks              []string `json:"hooks,omitempty"`
	CharacterPatterns  []string `json:"character_patterns,omitempty"`
	SettingPatterns    []string `json:"setting_patterns,omitempty"`
	ReusableTechniques []string `json:"reusable_techniques,omitempty"`
	AuthorizedAnchors  []string `json:"authorized_anchors,omitempty"`
	DoNotCopy          []string `json:"do_not_copy,omitempty"`
}

func ValidateBenchmarkName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("benchmark name is empty")
	}
	if !benchmarkNamePattern.MatchString(name) {
		return fmt.Errorf("invalid benchmark name %q", name)
	}
	return nil
}

func ValidateBenchmark(b *Benchmark) error {
	if b == nil {
		return fmt.Errorf("benchmark is nil")
	}
	if b.Version != BenchmarkProfileVersion {
		return fmt.Errorf("unsupported benchmark version %q", b.Version)
	}
	return ValidateBenchmarkName(b.Name)
}

func MarshalBenchmark(b Benchmark) ([]byte, error) {
	if b.Version == "" {
		b.Version = BenchmarkProfileVersion
	}
	return json.MarshalIndent(b, "", "  ")
}

func CompactBenchmark(b *Benchmark) *BenchmarkCompact {
	if b == nil {
		return nil
	}
	compact := b.BenchmarkCompact
	compact.Structure = compactBenchmarkItems(b.Structure)
	compact.Pacing = compactBenchmarkItems(b.Pacing)
	compact.Hooks = compactBenchmarkItems(b.Hooks)
	compact.CharacterPatterns = compactBenchmarkItems(b.CharacterPatterns)
	compact.SettingPatterns = compactBenchmarkItems(b.SettingPatterns)
	compact.ReusableTechniques = compactBenchmarkItems(b.ReusableTechniques)
	compact.AuthorizedAnchors = compactBenchmarkItems(b.AuthorizedAnchors)
	compact.DoNotCopy = compactBenchmarkItems(b.DoNotCopy)
	return &compact
}

func CompactBenchmarks(benchmarks []*Benchmark) []BenchmarkCompact {
	if len(benchmarks) == 0 {
		return nil
	}
	limit := min(len(benchmarks), maxCompactBenchmarkSummaries)
	out := make([]BenchmarkCompact, 0, limit)
	for i := range limit {
		if compact := CompactBenchmark(benchmarks[i]); compact != nil {
			out = append(out, *compact)
		}
	}
	return out
}

func compactBenchmarkItems(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), maxCompactBenchmarkItems)
	out := make([]string, limit)
	copy(out, items[:limit])
	return out
}
