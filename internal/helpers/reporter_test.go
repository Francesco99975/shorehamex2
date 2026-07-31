// reporter_test.go
package helpers

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createFileWithModTime(tb testing.TB, path string, modTime time.Time) {
	tb.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	require.NoError(tb, err)
	require.NoError(tb, f.Close())
	require.NoError(tb, os.Chtimes(path, modTime, modTime))
}

func parseLogLine(tb testing.TB, line string) (time.Time, SeverityType, string) {
	tb.Helper()
	line = strings.TrimSpace(line)
	if line == "" {
		tb.Fatal("empty log line")
	}

	parts := strings.SplitN(line, " [", 2)
	require.Len(tb, parts, 2)

	ts, err := time.ParseInLocation("2006-01-02 15:04:05", parts[0], time.Local)
	require.NoError(tb, err, "failed to parse timestamp")

	levelAndMsg := strings.SplitN(parts[1], "] ", 2)
	require.Len(tb, levelAndMsg, 2)

	level := SeverityType(strings.TrimSuffix(levelAndMsg[0], "]"))
	return ts, level, levelAndMsg[1]
}

func TestNewReporter_CreatesFileAndDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "app.log")

	r, err := NewReporter(path)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	assert.FileExists(t, path)
}

func TestReporter_Report_WritesCorrectFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	testCases := []struct {
		level SeverityType
		msg   string
	}{
		{SeverityLevels.PANIC, "system failure"},
		{SeverityLevels.ERROR, "disk error"},
		{SeverityLevels.WARN, "low memory"},
		{SeverityLevels.INFO, "user logged in"},
		{SeverityLevels.DEBUG, "debug trace"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(string(tc.level), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, fmt.Sprintf("log_%s.log", tc.level))
			r, err := NewReporter(path)
			require.NoError(t, err)
			defer func() { _ = r.Close() }()

			require.NoError(t, r.Report(tc.level, tc.msg))

			content, _ := os.ReadFile(path)
			line := strings.TrimSpace(string(content))
			ts, level, msg := parseLogLine(t, line)

			assert.Equal(t, tc.level, level)
			assert.Equal(t, tc.msg, msg)

			// FINAL FIX: Compare in the same location
			nowInLogLocation := time.Now().In(ts.Location())
			assert.WithinDuration(t, nowInLogLocation, ts, 3*time.Second, "timestamp should be recent")
		})
	}
}

func TestReporter_Report_ConcurrentSafety(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "race.log")
	r, err := NewReporter(path)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = r.Report(SeverityLevels.INFO, fmt.Sprintf("g%d-%d", id, j))
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

	file, _ := os.Open(path)
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	assert.Equal(t, 100*100, count)
}

func TestReporter_Close_PreventsFurtherWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, _ := NewReporter(filepath.Join(dir, "close.log"))
	require.NoError(t, r.Close())

	err := r.Report(SeverityLevels.ERROR, "after close")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reporter is closed")
}

func TestReporter_Cleanup_RemovesOldFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, _ := NewReporter(filepath.Join(dir, "app.log"))
	defer func() { _ = r.Close() }()

	now := time.Now()
	for _, name := range []string{"old1.log", "old2.log"} {
		createFileWithModTime(t, filepath.Join(dir, name), now.Add(-48*time.Hour))
	}
	for _, name := range []string{"keep1.log", "keep2.log"} {
		createFileWithModTime(t, filepath.Join(dir, name), now.Add(-6*time.Hour))
	}

	orig := r.filePath
	r.filePath = dir
	defer func() { r.filePath = orig }()
	r.Cleanup(24 * time.Hour)

	assert.NoFileExists(t, filepath.Join(dir, "old1.log"))
	assert.FileExists(t, filepath.Join(dir, "keep1.log"))
}

func TestReporter_Cleanup_IgnoresDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, _ := NewReporter(filepath.Join(dir, "log.log"))
	defer func() { _ = r.Close() }()

	sub := filepath.Join(dir, "archive")
	_ = os.Mkdir(sub, 0o755)
	createFileWithModTime(t, filepath.Join(sub, "old.log"), time.Now().Add(-1000*time.Hour))

	orig := r.filePath
	r.filePath = dir
	r.Cleanup(1 * time.Hour)
	r.filePath = orig

	assert.DirExists(t, sub)
}

// ——————————————————— BENCHMARKS ———————————————————

func BenchmarkReporter_Report(b *testing.B) {
	dir := b.TempDir()
	r, _ := NewReporter(filepath.Join(dir, "bench.log"))
	defer func() { _ = r.Close() }()

	for b.Loop() {
		_ = r.Report(SeverityLevels.INFO, "bench")
	}
}
