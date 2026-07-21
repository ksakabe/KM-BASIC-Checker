package checker

type Options struct {
	Target      string
	LibraryDirs []string
}

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
}

type FileResult struct {
	Path        string       `json:"path"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (r FileResult) ErrorCount() int {
	n := 0
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			n++
		}
	}
	return n
}

func (r FileResult) WarningCount() int {
	n := 0
	for _, d := range r.Diagnostics {
		if d.Severity == Warning {
			n++
		}
	}
	return n
}
