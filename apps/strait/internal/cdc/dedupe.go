package cdc

import "sync"

type recentDedupe struct {
	mu    sync.Mutex
	limit int
	seen  map[string]struct{}
	order []string
}

func newRecentDedupe(limit int) *recentDedupe {
	if limit <= 0 {
		limit = 1
	}
	return &recentDedupe{
		limit: limit,
		seen:  make(map[string]struct{}, limit),
		order: make([]string, 0, limit),
	}
}

func (d *recentDedupe) Remember(key string) bool {
	if d == nil || key == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.seen[key]; ok {
		return false
	}
	d.seen[key] = struct{}{}
	d.order = append(d.order, key)
	for len(d.order) > d.limit {
		evicted := d.order[0]
		delete(d.seen, evicted)
		copy(d.order, d.order[1:])
		d.order = d.order[:len(d.order)-1]
	}
	return true
}

func (d *recentDedupe) Forget(key string) {
	if d == nil || key == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.seen[key]; !ok {
		return
	}
	delete(d.seen, key)
	for i, existing := range d.order {
		if existing == key {
			copy(d.order[i:], d.order[i+1:])
			d.order = d.order[:len(d.order)-1]
			return
		}
	}
}
