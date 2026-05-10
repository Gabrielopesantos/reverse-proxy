package utilities

import (
	"sync"
)

type void struct{}

type Set[T comparable] struct {
	items map[T]void
	mu    sync.Mutex
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		items: make(map[T]void, 0),
	}
}

func (s *Set[T]) Add(item T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, inserted := s.items[item]
	if inserted {
		return
	}

	s.items[item] = void{}
}

func (s *Set[T]) Remove(item T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, item)
}

func (s *Set[T]) Has(item T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, added := s.items[item]
	return added
}

func (s *Set[T]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.items)
}

func (s *Set[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.items {
		delete(s.items, k)
	}
}
