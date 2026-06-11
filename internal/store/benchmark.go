package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/domain"
)

type BenchmarkStore struct{ io *IO }

func NewBenchmarkStore(io *IO) *BenchmarkStore { return &BenchmarkStore{io: io} }

func (s *BenchmarkStore) filePath(name string) (string, error) {
	if err := domain.ValidateBenchmarkName(name); err != nil {
		return "", err
	}
	return filepath.Join("meta", "benchmarks", name+".json"), nil
}

func (s *BenchmarkStore) Save(b domain.Benchmark) error {
	if b.Version == "" {
		b.Version = domain.BenchmarkProfileVersion
	}
	if err := domain.ValidateBenchmark(&b); err != nil {
		return err
	}
	rel, err := s.filePath(b.Name)
	if err != nil {
		return err
	}
	return s.io.WriteJSON(rel, b)
}

func (s *BenchmarkStore) Load(name string) (*domain.Benchmark, error) {
	rel, err := s.filePath(name)
	if err != nil {
		return nil, err
	}
	var benchmark domain.Benchmark
	if err := s.io.ReadJSON(rel, &benchmark); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := domain.ValidateBenchmark(&benchmark); err != nil {
		return nil, err
	}
	return &benchmark, nil
}

func (s *BenchmarkStore) LoadAll() ([]*domain.Benchmark, error) {
	entries, err := os.ReadDir(filepath.Join(s.io.dir, "meta", "benchmarks"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	benchmarks := make([]*domain.Benchmark, 0, len(entries))
	var loadErrs []error
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		benchmark, err := s.Load(name)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Errorf("load benchmark %s: %w", entry.Name(), err))
			continue
		}
		if benchmark != nil {
			benchmarks = append(benchmarks, benchmark)
		}
	}
	return benchmarks, errors.Join(loadErrs...)
}

func (s *BenchmarkStore) LoadSummaries() ([]domain.BenchmarkCompact, error) {
	benchmarks, err := s.LoadAll()
	if len(benchmarks) == 0 {
		return nil, err
	}
	return domain.CompactBenchmarks(benchmarks), err
}
