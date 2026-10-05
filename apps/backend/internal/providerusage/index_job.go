package providerusage

import (
	"context"
	"io"
	"os"
	"sync"
	"time"
)

// Index states.
const (
	IndexNever   = "never"
	IndexRunning = "running"
	IndexDone    = "done"
	IndexFailed  = "failed"
)

const (
	ledgerPageSize       = 5000
	observationRetention = 90 * 24 * time.Hour
)

// IndexStatus is the index job's observable progress.
type IndexStatus struct {
	State      string    `json:"state"`
	FilesTotal int       `json:"files_total"`
	FilesDone  int       `json:"files_done"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ErrorCode  string    `json:"error_code,omitempty"`
}

// Indexer runs the single-flight history index. The Kandev ledger is always
// scanned; local scanners run only when their source is enabled.
type Indexer struct {
	repo     *Repository
	ledger   LedgerReader
	attr     *attributor
	scanners []Scanner

	mu             sync.Mutex
	cancel         context.CancelFunc
	status         IndexStatus
	running        bool
	rerunRequested bool
	// done closes when the current run goroutine exits.
	done chan struct{}
}

// NewIndexer builds an indexer over the repository and ledger reader.
func NewIndexer(repo *Repository, ledger LedgerReader, resolver AccountResolver, accounts Accounts, scanners []Scanner) *Indexer {
	return &Indexer{
		repo:     repo,
		ledger:   ledger,
		attr:     &attributor{accounts: accounts, resolver: resolver},
		scanners: scanners,
		status:   IndexStatus{State: IndexNever},
	}
}

// NewDefaultIndexer wires the host scanners for home.
func NewDefaultIndexer(repo *Repository, ledger LedgerReader, resolver AccountResolver, home string) *Indexer {
	return NewIndexer(repo, ledger, resolver, HostAccounts(home), DefaultScanners(home))
}

// DefaultScanners returns the opt-in local scanners for home.
func DefaultScanners(home string) []Scanner {
	return []Scanner{
		NewClaudeScanner(home),
		NewCodexScanner(home),
		NewAntigravityScanner(home),
	}
}

// Status returns a copy of the current index status.
func (idx *Indexer) Status() IndexStatus {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.status
}

// Start launches the job unless one is already running. A start while running
// records a rerun request so a newly enabled source is indexed once the current
// pass finishes, rather than waiting for a manual Re-index.
func (idx *Indexer) Start() IndexStatus {
	idx.mu.Lock()
	if idx.running {
		idx.rerunRequested = true
		status := idx.status
		idx.mu.Unlock()
		return status
	}
	ctx, cancel := context.WithCancel(context.Background())
	idx.cancel = cancel
	idx.running = true
	idx.rerunRequested = false
	idx.done = make(chan struct{})
	idx.status = IndexStatus{State: IndexRunning, StartedAt: time.Now().UTC()}
	status := idx.status
	idx.mu.Unlock()

	go idx.run(ctx)
	return status
}

// Stop cancels a running job without waiting for it.
func (idx *Indexer) Stop() {
	idx.mu.Lock()
	cancel := idx.cancel
	idx.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StopAndWait cancels a running job and waits for its goroutine to exit (or ctx
// to be done). It is the barrier a source disable uses so the deletion cannot
// race a still-running insert for that source. It reports whether a run was
// actually interrupted, so the caller can restart it for the remaining sources.
func (idx *Indexer) StopAndWait(ctx context.Context) bool {
	idx.mu.Lock()
	running := idx.running
	cancel := idx.cancel
	done := idx.done
	idx.mu.Unlock()
	if !running {
		return false
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return true
}

func (idx *Indexer) markStopped() {
	idx.mu.Lock()
	idx.running = false
	idx.cancel = nil
	done := idx.done
	idx.done = nil
	idx.mu.Unlock()
	if done != nil {
		close(done)
	}
}

func (idx *Indexer) consumeRerun() bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	rerun := idx.rerunRequested
	idx.rerunRequested = false
	return rerun
}

func (idx *Indexer) resetRunning() {
	idx.mu.Lock()
	idx.status = IndexStatus{State: IndexRunning, StartedAt: time.Now().UTC()}
	idx.mu.Unlock()
}

func (idx *Indexer) sourceEnabled(ctx context.Context, source string) bool {
	if source == SourceKandev {
		return true
	}
	sources, err := idx.repo.ListSources(ctx)
	if err != nil {
		return false
	}
	return SourceEnabled(sources, source)
}

type fileWork struct {
	scanner Scanner
	path    string
}

func (idx *Indexer) run(ctx context.Context) {
	defer idx.markStopped()
	for {
		idx.finish(idx.runPass(ctx))
		if ctx.Err() != nil || !idx.consumeRerun() {
			return
		}
		idx.resetRunning()
	}
}

func (idx *Indexer) runPass(ctx context.Context) string {
	errorCode := ""
	sources, err := idx.repo.ListSources(ctx)
	if err != nil {
		errorCode = "sources_unreadable"
	}
	idx.attr.resetRunCache()
	work := idx.discoverWork(sources)
	idx.setTotal(1 + len(work))

	done := 0
	if err := idx.indexKandev(ctx); err != nil && ctx.Err() == nil {
		errorCode = "ledger_unreadable"
	}
	done++
	idx.setDone(done)

	for _, item := range work {
		if ctx.Err() != nil {
			break
		}
		idx.indexFile(ctx, item)
		done++
		idx.setDone(done)
	}

	// Pruning is best-effort and never fails the run.
	_ = idx.repo.Prune(ctx, time.Now().UTC().Add(-observationRetention))
	// A pass cut short by a disable is not a completed run; report it so the
	// restarted pass (see Service.SetSource) owns the final `done`.
	if ctx.Err() != nil {
		return "cancelled"
	}
	return errorCode
}

func (idx *Indexer) discoverWork(sources map[string]Source) []fileWork {
	var work []fileWork
	for _, scanner := range idx.scanners {
		if !SourceEnabled(sources, scanner.Source()) {
			continue
		}
		files, err := scanner.ListFiles()
		if err != nil {
			continue
		}
		for _, path := range files {
			work = append(work, fileWork{scanner: scanner, path: path})
		}
	}
	return work
}

func (idx *Indexer) indexKandev(ctx context.Context) error {
	cursor, ok, err := idx.repo.GetCursor(ctx, KandevLedgerCursorKey)
	if err != nil {
		return err
	}
	lastID := int64(0)
	if ok {
		lastID = cursor.ByteOffset
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		events, err := idx.ledger.ListUsageEventsAfter(ctx, lastID, ledgerPageSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		if err := idx.repo.InsertObservations(ctx, idx.attr.kandevObservations(ctx, events)); err != nil {
			return err
		}
		lastID = events[len(events)-1].ID
		if err := idx.repo.PutCursor(ctx, FileCursor{
			PathHash:   KandevLedgerCursorKey,
			Source:     SourceKandev,
			ByteOffset: lastID,
			UpdatedAt:  time.Now().UTC(),
		}); err != nil {
			return err
		}
		if len(events) < ledgerPageSize {
			return nil
		}
	}
}

func (idx *Indexer) indexFile(ctx context.Context, item fileWork) {
	// A source disabled mid-run must not be re-indexed: the disable deletes
	// its observations and cursors, and a later insert would resurrect them.
	if !idx.sourceEnabled(ctx, item.scanner.Source()) {
		return
	}
	info, err := os.Stat(item.path)
	if err != nil {
		return
	}
	hash := pathHash(item.path)
	cursor, ok, err := idx.repo.GetCursor(ctx, hash)
	if err != nil {
		return
	}
	if ok && cursor.Size == info.Size() && cursor.Mtime.Equal(info.ModTime()) {
		return
	}
	offset := resumeOffset(cursor, ok, info.Size())
	observations, consumed, err := idx.parseFile(item, offset)
	if err != nil {
		return
	}
	// Re-check at the write boundary: a disable that landed while this file
	// was parsing still wins.
	if !idx.sourceEnabled(ctx, item.scanner.Source()) {
		return
	}
	if err := idx.repo.InsertObservations(ctx, observations); err != nil {
		return
	}
	_ = idx.repo.PutCursor(ctx, FileCursor{
		PathHash:   hash,
		Source:     item.scanner.Source(),
		Size:       info.Size(),
		Mtime:      info.ModTime(),
		ByteOffset: offset + consumed,
		UpdatedAt:  time.Now().UTC(),
	})
}

func resumeOffset(cursor FileCursor, ok bool, size int64) int64 {
	if ok && cursor.ByteOffset > 0 && cursor.ByteOffset <= size {
		return cursor.ByteOffset
	}
	return 0
}

func (idx *Indexer) parseFile(item fileWork, offset int64) ([]Observation, int64, error) {
	file, err := os.Open(item.path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return nil, 0, err
		}
	}
	return item.scanner.Parse(file, item.path)
}

func (idx *Indexer) setTotal(total int) {
	idx.mu.Lock()
	idx.status.FilesTotal = total
	idx.mu.Unlock()
}

func (idx *Indexer) setDone(done int) {
	idx.mu.Lock()
	idx.status.FilesDone = done
	idx.mu.Unlock()
}

func (idx *Indexer) finish(errorCode string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.status.FinishedAt = time.Now().UTC()
	idx.status.ErrorCode = errorCode
	if errorCode == "" {
		idx.status.State = IndexDone
	} else {
		idx.status.State = IndexFailed
	}
}
