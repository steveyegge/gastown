package beads

// BdSemaphore limits concurrent bd process execution.
//
// Embedded dolt uses file-level locking; concurrent bd processes contend for
// the lock and hang indefinitely when dozens run at once (e.g., gt status
// spawns 40+ bd processes). This semaphore serializes access so at most
// MaxConcurrentBd processes run simultaneously.
//
// The channel-based semaphore allows goroutines to queue and wait their turn
// rather than failing, preserving the existing parallel architecture while
// eliminating lock contention.
var BdSemaphore = make(chan struct{}, MaxConcurrentBd)

// MaxConcurrentBd is the maximum number of bd processes that can run at once.
// Set to 3: allows some parallelism across different dolt DBs while preventing
// the 40+ process pile-up that causes indefinite hangs.
const MaxConcurrentBd = 3

// AcquireBd blocks until a bd execution slot is available.
func AcquireBd() {
	BdSemaphore <- struct{}{}
}

// ReleaseBd releases a bd execution slot.
func ReleaseBd() {
	<-BdSemaphore
}
