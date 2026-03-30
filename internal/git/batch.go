package git

// BatchPusher pushes accumulated commits at a configurable interval.
type BatchPusher struct {
	git      *Git
	interval int  // push every N iterations
	counter  int  // iterations since last push
	skipPush bool // when true, never actually push
}

// NewBatchPusher creates a BatchPusher that pushes every interval iterations.
// If skipPush is true, pushes are silently skipped.
func NewBatchPusher(git *Git, interval int, skipPush bool) *BatchPusher {
	return &BatchPusher{
		git:      git,
		interval: interval,
		skipPush: skipPush,
	}
}

// AfterIteration increments the counter and pushes when the threshold is
// reached. It returns whether a push actually occurred.
func (bp *BatchPusher) AfterIteration() (pushed bool, err error) {
	bp.counter++
	if bp.counter >= bp.interval {
		if err := bp.push(); err != nil {
			return false, err
		}
		bp.Reset()
		return true, nil
	}
	return false, nil
}

// Flush forces a push of any remaining commits.
func (bp *BatchPusher) Flush() error {
	if bp.counter == 0 {
		return nil
	}
	err := bp.push()
	if err == nil {
		bp.Reset()
	}
	return err
}

// Reset zeroes the iteration counter.
func (bp *BatchPusher) Reset() {
	bp.counter = 0
}

func (bp *BatchPusher) push() error {
	if bp.skipPush {
		return nil
	}
	return bp.git.Push()
}
