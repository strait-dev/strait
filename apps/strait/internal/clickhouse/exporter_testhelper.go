package clickhouse

// NewTestExporter creates a minimal Exporter suitable for unit tests.
// It has no client or flush loop, but Enqueue will buffer records.
func NewTestExporter() *Exporter {
	return &Exporter{
		config:  ExporterConfig{BatchSize: 1000, Enabled: true},
		pending: make([]any, 0, 64),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// PendingLen returns the number of buffered records. Test-only.
func (e *Exporter) PendingLen() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pending)
}

// PendingAt returns the buffered record at index i. Test-only.
func (e *Exporter) PendingAt(i int) any {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if i < 0 || i >= len(e.pending) {
		return nil
	}
	return e.pending[i]
}
