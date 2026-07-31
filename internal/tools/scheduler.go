// cron.go
package tools

import (
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

type Job struct {
	ID      string
	EntryID cron.EntryID
}

var (
	cronScheduler *cron.Cron
	jobs          = make(map[string]Job)
	jobMutex      sync.Mutex
	once          sync.Once
)

func InitCron() {
	once.Do(func() {
		slog.Info("Initializing cron scheduler")
		cronScheduler = cron.New(cron.WithSeconds()) // ← seconds enabled!
		cronScheduler.Start()
	})
}

func AddJob(id string, schedule string, task func()) error {
	jobMutex.Lock()
	defer jobMutex.Unlock()

	if _, exists := jobs[id]; exists {
		slog.Warn("Job with ID already exists", slog.String("id", id))
		return nil
	}

	entryID, err := cronScheduler.AddFunc(schedule, task)
	if err != nil {
		return err
	}

	jobs[id] = Job{
		ID:      id,
		EntryID: entryID,
	}
	slog.Info("Scheduled job ID", slog.String("id", id))
	return nil
}

func UpdateJob(id string, schedule string, task func()) error {
	jobMutex.Lock()
	defer jobMutex.Unlock()

	job, exists := jobs[id]
	if !exists {
		slog.Warn("Job with ID not found", slog.String("id", id))
		return nil
	}

	cronScheduler.Remove(job.EntryID)

	entryID, err := cronScheduler.AddFunc(schedule, task)
	if err != nil {
		return err
	}

	jobs[id] = Job{
		ID:      id,
		EntryID: entryID,
	}
	slog.Info("Updated job ID", slog.String("id", id))
	return nil
}

func RemoveJob(id string) {
	jobMutex.Lock()
	defer jobMutex.Unlock()

	job, ok := jobs[id]
	if !ok {
		slog.Warn("Job with ID not found", slog.String("id", id))
		return
	}

	cronScheduler.Remove(job.EntryID)
	delete(jobs, id)
	slog.Info("Removed job ID", slog.String("id", id))
}

// Graceful shutdown — call this in main() or tests
func ShutdownCron() {
	if cronScheduler != nil {
		slog.Info("Shutting down cron scheduler")
		cronScheduler.Stop()
	}
}
