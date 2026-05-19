package playwright

import (
	"encoding/json"
	"io"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

type report struct {
	Suites []suite `json:"suites"`
}

type suite struct {
	Title  string  `json:"title"`
	File   string  `json:"file"`
	Suites []suite `json:"suites"`
	Specs  []spec  `json:"specs"`
}

type spec struct {
	Title string `json:"title"`
	File  string `json:"file"`
	Tests []test `json:"tests"`
}

type test struct {
	Status  string   `json:"status"`
	Results []result `json:"results"`
}

type result struct {
	Status    string    `json:"status"`
	Duration  int       `json:"duration"`
	StartTime time.Time `json:"startTime"`
	Error     *pwError  `json:"error"`
}

type pwError struct {
	Message string `json:"message"`
}

// RawResult is one test attempt extracted from the Playwright JSON report.
type RawResult struct {
	TestID         string
	TestName       string
	Status         string
	DurationMS     int
	AttemptIndex   int
	FailureMessage string
	StartedAt      time.Time
}

// Parse reads a Playwright results.json and returns one RawResult per attempt.
// Retries are separate results so flakiness can be measured.
func Parse(r io.Reader) ([]RawResult, error) {
	var rep report
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return nil, err
	}
	var out []RawResult
	for _, s := range rep.Suites {
		collectSpecs(s, &out)
	}
	return out, nil
}

func collectSpecs(s suite, out *[]RawResult) {
	file := s.File
	for _, spec := range s.Specs {
		specFile := spec.File
		if specFile == "" {
			specFile = file
		}
		testID := specFile + "::" + spec.Title
		for _, t := range spec.Tests {
			for i, res := range t.Results {
				rr := RawResult{
					TestID:       testID,
					TestName:     spec.Title,
					Status:       event.NormalizeStatus(event.FrameworkPlaywright, res.Status),
					DurationMS:   res.Duration,
					AttemptIndex: i,
					StartedAt:    res.StartTime,
				}
				if res.Error != nil {
					rr.FailureMessage = event.TruncateExcerpt(res.Error.Message)
				}
				*out = append(*out, rr)
			}
		}
	}
	for _, child := range s.Suites {
		if child.File == "" {
			child.File = file
		}
		collectSpecs(child, out)
	}
}
