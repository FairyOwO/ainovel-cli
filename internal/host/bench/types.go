package bench

import "time"

type Options struct {
	SourceDir string
	Name      string
}

type Result struct {
	Name      string
	Title     string
	SourceDir string
	Files     int
	Path      string
}

type Stage string

const (
	StageScan  Stage = "scan"
	StageParse Stage = "parse"
	StageSave  Stage = "save"
	StageDone  Stage = "done"
	StageError Stage = "error"
)

type Event struct {
	Time    time.Time
	Stage   Stage
	Current int
	Total   int
	Message string
	Err     error
}
