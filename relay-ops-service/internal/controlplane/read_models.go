package controlplane

import "sync"

type MemoryReader struct {
	mu     sync.RWMutex
	Models map[string]ReadModel
}

func NewMemoryReader() *MemoryReader { return &MemoryReader{Models: map[string]ReadModel{}} }
func (r *MemoryReader) Read(name string, _ map[string]string) (ReadModel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.Models[name]
	if !ok {
		return ReadModel{Items: []any{}, Freshness: Freshness{Completeness: "empty"}}, nil
	}
	return m, nil
}
func (r *MemoryReader) Set(name string, m ReadModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Models[name] = m
}
