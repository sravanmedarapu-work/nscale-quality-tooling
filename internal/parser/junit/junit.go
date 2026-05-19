package junit

import (
	"encoding/xml"
	"io"
	"strings"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

type testSuites struct {
	XMLName    xml.Name    `xml:"testsuites"`
	TestSuites []testSuite `xml:"testsuite"`
}

type testSuite struct {
	XMLName   xml.Name   `xml:"testsuite"`
	Name      string     `xml:"name,attr"`
	TestCases []testCase `xml:"testcase"`
}

type testCase struct {
	Name      string   `xml:"name,attr"`
	Classname string   `xml:"classname,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   *failure `xml:"failure"`
	Error     *failure `xml:"error"`
	Skipped   *skipped `xml:"skipped"`
}

type failure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type skipped struct{}

// Parse reads JUnit XML from r and returns raw test cases grouped by suite name.
// Each testcase element is treated as one attempt.
func Parse(r io.Reader) ([]RawCase, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Try <testsuites> root first, fall back to bare <testsuite>
	// Try <testsuites> root first; fall back to bare <testsuite>.
	var suites testSuites
	if xmlErr := xml.Unmarshal(data, &suites); xmlErr != nil || len(suites.TestSuites) == 0 {
		var single testSuite
		if err2 := xml.Unmarshal(data, &single); err2 != nil {
			if xmlErr != nil {
				return nil, xmlErr
			}
			return nil, err2
		}
		if single.Name != "" || len(single.TestCases) > 0 {
			suites.TestSuites = []testSuite{single}
		}
	}

	var out []RawCase
	for _, ts := range suites.TestSuites {
		for _, tc := range ts.TestCases {
			rc := RawCase{
				SuiteName:  ts.Name,
				Classname:  tc.Classname,
				Name:       tc.Name,
				DurationMS: int(tc.Time * 1000),
				StartedAt:  time.Now().UTC(),
			}
			switch {
			case tc.Skipped != nil:
				rc.Status = event.StatusSkipped
			case tc.Failure != nil:
				rc.Status = event.StatusFailed
				msg := strings.TrimSpace(tc.Failure.Message)
				if msg == "" {
					msg = strings.TrimSpace(tc.Failure.Body)
				}
				rc.FailureMessage = event.TruncateExcerpt(msg)
			case tc.Error != nil:
				rc.Status = event.StatusFailed
				msg := strings.TrimSpace(tc.Error.Message)
				if msg == "" {
					msg = strings.TrimSpace(tc.Error.Body)
				}
				rc.FailureMessage = event.TruncateExcerpt(msg)
			default:
				rc.Status = event.StatusPassed
			}
			// test_id: classname.name (JUnit convention)
			if rc.Classname != "" {
				rc.TestID = rc.Classname + "::" + rc.Name
			} else {
				rc.TestID = rc.Name
			}
			out = append(out, rc)
		}
	}
	return out, nil
}

// RawCase holds parsed fields before normalization.
type RawCase struct {
	SuiteName      string
	TestID         string
	Classname      string
	Name           string
	Status         string
	DurationMS     int
	FailureMessage string
	StartedAt      time.Time
}
