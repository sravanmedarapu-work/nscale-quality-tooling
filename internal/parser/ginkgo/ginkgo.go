package ginkgo

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

// Ginkgo v2 JSON report is an array of suite reports.
type suiteReport struct {
	SuiteDescription string       `json:"SuiteDescription"`
	SpecReports      []specReport `json:"SpecReports"`
}

type specReport struct {
	ContainerHierarchyTexts []string       `json:"ContainerHierarchyTexts"`
	LeafNodeType            string         `json:"LeafNodeType"`
	LeafNodeText            string         `json:"LeafNodeText"`
	State                   string         `json:"State"`
	RunTime                 int64          `json:"RunTime"` // nanoseconds
	NumAttempts             int            `json:"NumAttempts"`
	StartTime               time.Time      `json:"StartTime"`
	Failure                 *ginkgoFailure `json:"Failure"`
}

type ginkgoFailure struct {
	Message string `json:"Message"`
}

// RawResult is one test attempt from a Ginkgo report.
type RawResult struct {
	TestID         string
	TestName       string
	Status         string
	DurationMS     int
	AttemptIndex   int
	FailureMessage string
	StartedAt      time.Time
}

// Parse reads a Ginkgo v2 JSON report (array of suite reports) and returns
// one RawResult per spec attempt.
func Parse(r io.Reader) ([]RawResult, error) {
	var reports []suiteReport
	if err := json.NewDecoder(r).Decode(&reports); err != nil {
		return nil, err
	}
	var out []RawResult
	for _, sr := range reports {
		for _, spec := range sr.SpecReports {
			// Skip setup/teardown nodes (BeforeSuite, AfterSuite, etc.) — they
			// have no LeafNodeText and are not addressable test cases.
			if spec.LeafNodeText == "" {
				continue
			}
			testID := buildTestID(spec.ContainerHierarchyTexts, spec.LeafNodeText)
			attempts := spec.NumAttempts
			if attempts < 1 {
				attempts = 1
			}
			for i := 0; i < attempts; i++ {
				status := event.NormalizeStatus(event.FrameworkGinkgo, strings.ToLower(spec.State))
				// For retried specs treat all but last as failed, last as the reported state
				if attempts > 1 && i < attempts-1 {
					status = event.StatusFailed
				}
				rr := RawResult{
					TestID:       testID,
					TestName:     spec.LeafNodeText,
					Status:       status,
					DurationMS:   int(spec.RunTime / 1_000_000),
					AttemptIndex: i,
					StartedAt:    spec.StartTime,
				}
				if spec.Failure != nil && status == event.StatusFailed {
					rr.FailureMessage = event.TruncateExcerpt(spec.Failure.Message)
				}
				out = append(out, rr)
			}
		}
	}
	return out, nil
}

func buildTestID(containers []string, leaf string) string {
	if len(containers) == 0 {
		return leaf
	}
	return strings.Join(containers, " > ") + "::" + leaf
}
