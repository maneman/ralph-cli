package telemetry

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
)

// computeDelta returns a Delta comparing current against baseline.
// higherIsBetter controls the symbol direction: when true, current > baseline
// is ▲ (improvement); when false, current > baseline is ▼ (regression).
func computeDelta(current, baseline float64, higherIsBetter bool) Delta {
	diff := current - baseline
	symbol := "─"

	const epsilon = 1e-9
	if math.Abs(diff) > epsilon {
		if (diff > 0) == higherIsBetter {
			symbol = "▲"
		} else {
			symbol = "▼"
		}
	}

	return Delta{
		Current:  current,
		Baseline: baseline,
		Diff:     diff,
		Symbol:   symbol,
	}
}

// LoadBaseline reads the last windowSize run summaries from runsFile and
// computes a rolling-average baseline comparison. Returns nil with no error
// if the file does not exist or contains fewer than 1 run.
func LoadBaseline(runsFile string, windowSize int) (*BaselineComparison, error) {
	runs, err := readRunSummaries(runsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if len(runs) == 0 {
		return nil, nil
	}

	// Take the last windowSize entries.
	if len(runs) > windowSize {
		runs = runs[len(runs)-windowSize:]
	}

	n := float64(len(runs))
	var sumSuccessRate, sumGapRate, sumDuration, sumRetries float64
	for _, r := range runs {
		sumSuccessRate += r.SuccessRate
		sumGapRate += r.GapRate
		sumDuration += float64(r.TotalTime)
		sumRetries += r.AvgRetries
	}

	return &BaselineComparison{
		SuccessRate: Delta{Baseline: sumSuccessRate / n},
		GapRate:     Delta{Baseline: sumGapRate / n},
		AvgDuration: Delta{Baseline: sumDuration / n},
		AvgRetries:  Delta{Baseline: sumRetries / n},
	}, nil
}

// ComputeBaseline compares the current run against the rolling average of
// past runs stored in runsFile. Returns nil if no baseline data is available.
func ComputeBaseline(current RunSummary, runsFile string) *BaselineComparison {
	runs, err := readRunSummaries(runsFile)
	if err != nil || len(runs) == 0 {
		return nil
	}

	// Default window of 5 for comparison.
	const windowSize = 5
	if len(runs) > windowSize {
		runs = runs[len(runs)-windowSize:]
	}

	n := float64(len(runs))
	var sumSuccessRate, sumGapRate, sumDuration, sumRetries float64
	for _, r := range runs {
		sumSuccessRate += r.SuccessRate
		sumGapRate += r.GapRate
		sumDuration += float64(r.TotalTime)
		sumRetries += r.AvgRetries
	}

	avgSuccessRate := sumSuccessRate / n
	avgGapRate := sumGapRate / n
	avgDuration := sumDuration / n
	avgRetries := sumRetries / n

	return &BaselineComparison{
		SuccessRate: computeDelta(current.SuccessRate, avgSuccessRate, true),
		GapRate:     computeDelta(current.GapRate, avgGapRate, false), // lower gap rate is better
		AvgDuration: computeDelta(float64(current.TotalTime), avgDuration, false), // lower duration is better
		AvgRetries:  computeDelta(current.AvgRetries, avgRetries, false), // lower retries is better
	}
}

// readRunSummaries reads all RunSummary records from a JSONL file.
func readRunSummaries(path string) ([]RunSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var runs []RunSummary
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rs RunSummary
		if err := json.Unmarshal(line, &rs); err != nil {
			continue // skip malformed lines
		}
		runs = append(runs, rs)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}
