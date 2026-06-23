package queue

const (
	JobsStream    = "jobs"
	ResultsStream = "results"

	WorkerGroup  = "workers"    // consumes jobs
	BackendGroup = "go-backend" // consumes results
)
