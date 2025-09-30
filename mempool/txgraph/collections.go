package txgraph

import (
	"container/heap"
	"iter"
)

// Stack implements a generic LIFO stack data structure with O(1) Push and Pop.
// The zero value is ready to use.
type Stack[T any] struct {
	items []T
}

// NewStack creates a new empty stack with optional initial capacity.
func NewStack[T any](capacity ...int) *Stack[T] {
	cap := 0
	if len(capacity) > 0 {
		cap = capacity[0]
	}
	return &Stack[T]{
		items: make([]T, 0, cap),
	}
}

// Push adds an item to the top of the stack.
func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

// Pop removes and returns the item at the top of the stack.
// Returns false if the stack is empty.
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	idx := len(s.items) - 1
	item := s.items[idx]
	s.items = s.items[:idx]
	return item, true
}

// Peek returns the item at the top of the stack without removing it.
// Returns false if the stack is empty.
func (s *Stack[T]) Peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

// Len returns the number of items in the stack.
func (s *Stack[T]) Len() int {
	return len(s.items)
}

// IsEmpty returns true if the stack contains no items.
func (s *Stack[T]) IsEmpty() bool {
	return len(s.items) == 0
}

// Clear removes all items from the stack.
func (s *Stack[T]) Clear() {
	s.items = s.items[:0]
}

// Iterate returns an iterator that yields items from top to bottom without
// modifying the stack.
func (s *Stack[T]) Iterate() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(s.items) - 1; i >= 0; i-- {
			if !yield(s.items[i]) {
				return
			}
		}
	}
}

// Queue implements a generic FIFO queue with amortized O(1) Enqueue and
// Dequeue operations. The zero value is ready to use.
type Queue[T any] struct {
	items []T
}

// NewQueue creates a new empty queue with optional initial capacity.
func NewQueue[T any](capacity ...int) *Queue[T] {
	cap := 0
	if len(capacity) > 0 {
		cap = capacity[0]
	}
	return &Queue[T]{
		items: make([]T, 0, cap),
	}
}

// Enqueue adds an item to the back of the queue.
func (q *Queue[T]) Enqueue(item T) {
	q.items = append(q.items, item)
}

// Dequeue removes and returns the item at the front of the queue.
// Returns false if the queue is empty.
func (q *Queue[T]) Dequeue() (T, bool) {
	if len(q.items) == 0 {
		var zero T
		return zero, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

// Peek returns the item at the front of the queue without removing it.
// Returns false if the queue is empty.
func (q *Queue[T]) Peek() (T, bool) {
	if len(q.items) == 0 {
		var zero T
		return zero, false
	}
	return q.items[0], true
}

// Len returns the number of items in the queue.
func (q *Queue[T]) Len() int {
	return len(q.items)
}

// IsEmpty returns true if the queue contains no items.
func (q *Queue[T]) IsEmpty() bool {
	return len(q.items) == 0
}

// Clear removes all items from the queue.
func (q *Queue[T]) Clear() {
	q.items = q.items[:0]
}

// Iterate returns an iterator that yields items from front to back without
// modifying the queue.
func (q *Queue[T]) Iterate() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, item := range q.items {
			if !yield(item) {
				return
			}
		}
	}
}

// PriorityQueue implements a generic priority queue using container/heap
// ordered by a comparison function. The zero value is NOT ready to use;
// use NewPriorityQueue to create an instance.
type PriorityQueue[T any] struct {
	impl *heapImpl[T]
}

// NewPriorityQueue creates a new priority queue with the given comparison
// function where less(a, b) returns true if a has higher priority than b.
// For a max-heap use: func(a, b) { return a > b }.
func NewPriorityQueue[T any](
	less func(a, b T) bool,
	capacity ...int,
) *PriorityQueue[T] {

	cap := 0
	if len(capacity) > 0 {
		cap = capacity[0]
	}
	return &PriorityQueue[T]{
		impl: &heapImpl[T]{
			items: make([]T, 0, cap),
			less:  less,
		},
	}
}

// Push adds an item to the priority queue.
func (pq *PriorityQueue[T]) Push(item T) {
	heap.Push(pq.impl, item)
}

// Pop removes and returns the highest priority item from the queue.
// Returns false if the queue is empty.
func (pq *PriorityQueue[T]) Pop() (T, bool) {
	if pq.impl.Len() == 0 {
		var zero T
		return zero, false
	}
	return heap.Pop(pq.impl).(T), true
}

// Peek returns the highest priority item without removing it.
// Returns false if the queue is empty.
func (pq *PriorityQueue[T]) Peek() (T, bool) {
	if pq.impl.Len() == 0 {
		var zero T
		return zero, false
	}
	return pq.impl.items[0], true
}

// Len returns the number of items in the priority queue.
func (pq *PriorityQueue[T]) Len() int {
	return pq.impl.Len()
}

// IsEmpty returns true if the priority queue contains no items.
func (pq *PriorityQueue[T]) IsEmpty() bool {
	return pq.impl.Len() == 0
}

// Clear removes all items from the priority queue.
func (pq *PriorityQueue[T]) Clear() {
	pq.impl.items = pq.impl.items[:0]
}

// Iterate returns an iterator that yields items in priority order by
// creating a temporary copy to avoid modifying the original queue.
func (pq *PriorityQueue[T]) Iterate() iter.Seq[T] {
	return func(yield func(T) bool) {
		tmpItems := make([]T, len(pq.impl.items))
		copy(tmpItems, pq.impl.items)
		tmp := &PriorityQueue[T]{
			impl: &heapImpl[T]{
				items: tmpItems,
				less:  pq.impl.less,
			},
		}

		for !tmp.IsEmpty() {
			item, _ := tmp.Pop()
			if !yield(item) {
				return
			}
		}
	}
}

// heapImpl implements heap.Interface to integrate with container/heap.
type heapImpl[T any] struct {
	items []T
	less  func(a, b T) bool
}

func (h *heapImpl[T]) Len() int {
	return len(h.items)
}

func (h *heapImpl[T]) Less(i, j int) bool {
	return h.less(h.items[i], h.items[j])
}

func (h *heapImpl[T]) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *heapImpl[T]) Push(x any) {
	h.items = append(h.items, x.(T))
}

func (h *heapImpl[T]) Pop() any {
	n := len(h.items) - 1
	item := h.items[n]
	h.items = h.items[:n]
	return item
}

var _ heap.Interface = (*heapImpl[int])(nil)