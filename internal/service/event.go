package service

type Stage string

const (
	StageResolve  Stage = "resolve"
	StageDownload Stage = "download"
	StageExtract  Stage = "extract"
	StageBackup   Stage = "backup"
	StageInstall  Stage = "install"
	StageDone     Stage = "done"
	StageCurrent  Stage = "up-to-date"
	StageInfo     Stage = "info"
	StageWarning  Stage = "warning"
	StageError    Stage = "error"
)

type Event struct {
	Ref     string
	Stage   Stage
	Message string
	Index   int
	Total   int
	Err     error
}

type EmitFunc func(Event)
