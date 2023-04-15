package utilities

import (
	"golang.org/x/exp/maps"
	"sync"
)

type void struct{}

type Set[T comparable] struct {
	sync.Mutex
	items map[T]void
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		items: make(map[T]void, 0),
	}
}

func (s *Set[T]) Add(item T) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	_, inserted := s.items[item]
	if inserted {
		return
	}

	s.items[item] = void{}
}

func (s *Set[T]) Remove(item T) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	delete(s.items, item)
}

func (s *Set[T]) Has(item T) bool {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	_, added := s.items[item]
	return added
}

func (s *Set[T]) Len() int {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	return len(s.items)
}

func (s *Set[T]) Clear() {
	maps.Clear(s.items)
}
