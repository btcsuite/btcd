package txgraph

import (
	"testing"
)

// TestStackBasicOperations verifies LIFO behavior and empty state handling.
func TestStackBasicOperations(t *testing.T) {
	t.Parallel()

	stack := NewStack[int]()

	if !stack.IsEmpty() {
		t.Error("New stack should be empty")
	}
	if stack.Len() != 0 {
		t.Errorf("Expected length 0, got %d", stack.Len())
	}

	_, ok := stack.Pop()
	if ok {
		t.Error("Pop on empty stack should return false")
	}

	_, ok = stack.Peek()
	if ok {
		t.Error("Peek on empty stack should return false")
	}

	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	if stack.Len() != 3 {
		t.Errorf("Expected length 3, got %d", stack.Len())
	}
	if stack.IsEmpty() {
		t.Error("Stack should not be empty")
	}

	val, ok := stack.Peek()
	if !ok || val != 3 {
		t.Errorf("Expected Peek to return 3, got %d", val)
	}
	if stack.Len() != 3 {
		t.Error("Peek should not modify stack size")
	}

	val, ok = stack.Pop()
	if !ok || val != 3 {
		t.Errorf("Expected Pop to return 3, got %d", val)
	}
	val, ok = stack.Pop()
	if !ok || val != 2 {
		t.Errorf("Expected Pop to return 2, got %d", val)
	}
	val, ok = stack.Pop()
	if !ok || val != 1 {
		t.Errorf("Expected Pop to return 1, got %d", val)
	}

	if !stack.IsEmpty() {
		t.Error("Stack should be empty after all pops")
	}
}

// TestStackIterate verifies iteration yields items top to bottom without
// modifying the stack state.
func TestStackIterate(t *testing.T) {
	t.Parallel()

	stack := NewStack[int]()
	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	expected := []int{3, 2, 1}
	idx := 0
	for val := range stack.Iterate() {
		if val != expected[idx] {
			t.Errorf(
				"Expected %d at index %d, got %d",
				expected[idx],
				idx,
				val,
			)
		}
		idx++
	}

	if idx != 3 {
		t.Errorf("Expected 3 iterations, got %d", idx)
	}

	if stack.Len() != 3 {
		t.Errorf(
			"Iteration should not modify stack, length is %d",
			stack.Len(),
		)
	}
}

// TestStackClear verifies Clear operation empties the stack.
func TestStackClear(t *testing.T) {
	t.Parallel()

	stack := NewStack[int]()
	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	stack.Clear()

	if !stack.IsEmpty() {
		t.Error("Stack should be empty after Clear")
	}
	if stack.Len() != 0 {
		t.Errorf("Expected length 0 after Clear, got %d", stack.Len())
	}
}

// TestQueueBasicOperations verifies FIFO behavior and empty state handling.
func TestQueueBasicOperations(t *testing.T) {
	t.Parallel()

	queue := NewQueue[int]()

	if !queue.IsEmpty() {
		t.Error("New queue should be empty")
	}
	if queue.Len() != 0 {
		t.Errorf("Expected length 0, got %d", queue.Len())
	}

	_, ok := queue.Dequeue()
	if ok {
		t.Error("Dequeue on empty queue should return false")
	}

	_, ok = queue.Peek()
	if ok {
		t.Error("Peek on empty queue should return false")
	}

	queue.Enqueue(1)
	queue.Enqueue(2)
	queue.Enqueue(3)

	if queue.Len() != 3 {
		t.Errorf("Expected length 3, got %d", queue.Len())
	}
	if queue.IsEmpty() {
		t.Error("Queue should not be empty")
	}

	val, ok := queue.Peek()
	if !ok || val != 1 {
		t.Errorf("Expected Peek to return 1, got %d", val)
	}
	if queue.Len() != 3 {
		t.Error("Peek should not modify queue size")
	}

	val, ok = queue.Dequeue()
	if !ok || val != 1 {
		t.Errorf("Expected Dequeue to return 1, got %d", val)
	}
	val, ok = queue.Dequeue()
	if !ok || val != 2 {
		t.Errorf("Expected Dequeue to return 2, got %d", val)
	}
	val, ok = queue.Dequeue()
	if !ok || val != 3 {
		t.Errorf("Expected Dequeue to return 3, got %d", val)
	}

	if !queue.IsEmpty() {
		t.Error("Queue should be empty after all dequeues")
	}
}

// TestQueueIterate verifies iteration yields items front to back without
// modifying the queue state.
func TestQueueIterate(t *testing.T) {
	t.Parallel()

	queue := NewQueue[int]()
	queue.Enqueue(1)
	queue.Enqueue(2)
	queue.Enqueue(3)

	expected := []int{1, 2, 3}
	idx := 0
	for val := range queue.Iterate() {
		if val != expected[idx] {
			t.Errorf(
				"Expected %d at index %d, got %d",
				expected[idx],
				idx,
				val,
			)
		}
		idx++
	}

	if idx != 3 {
		t.Errorf("Expected 3 iterations, got %d", idx)
	}

	if queue.Len() != 3 {
		t.Errorf(
			"Iteration should not modify queue, length is %d",
			queue.Len(),
		)
	}
}

// TestQueueClear verifies Clear operation empties the queue.
func TestQueueClear(t *testing.T) {
	t.Parallel()

	queue := NewQueue[int]()
	queue.Enqueue(1)
	queue.Enqueue(2)
	queue.Enqueue(3)

	queue.Clear()

	if !queue.IsEmpty() {
		t.Error("Queue should be empty after Clear")
	}
	if queue.Len() != 0 {
		t.Errorf("Expected length 0 after Clear, got %d", queue.Len())
	}
}

// TestPriorityQueueBasicOperations verifies max-heap behavior maintains
// priority ordering across Push/Pop operations.
func TestPriorityQueueBasicOperations(t *testing.T) {
	t.Parallel()

	pq := NewPriorityQueue(func(a, b int) bool {
		return a > b
	})

	if !pq.IsEmpty() {
		t.Error("New priority queue should be empty")
	}
	if pq.Len() != 0 {
		t.Errorf("Expected length 0, got %d", pq.Len())
	}

	_, ok := pq.Pop()
	if ok {
		t.Error("Pop on empty priority queue should return false")
	}

	_, ok = pq.Peek()
	if ok {
		t.Error("Peek on empty priority queue should return false")
	}

	pq.Push(3)
	pq.Push(1)
	pq.Push(4)
	pq.Push(2)

	if pq.Len() != 4 {
		t.Errorf("Expected length 4, got %d", pq.Len())
	}
	if pq.IsEmpty() {
		t.Error("Priority queue should not be empty")
	}

	val, ok := pq.Peek()
	if !ok || val != 4 {
		t.Errorf("Expected Peek to return 4, got %d", val)
	}
	if pq.Len() != 4 {
		t.Error("Peek should not modify priority queue size")
	}

	expected := []int{4, 3, 2, 1}
	for i, exp := range expected {
		val, ok := pq.Pop()
		if !ok {
			t.Errorf("Pop %d failed", i)
		}
		if val != exp {
			t.Errorf(
				"Expected Pop to return %d at position %d, got %d",
				exp,
				i,
				val,
			)
		}
	}

	if !pq.IsEmpty() {
		t.Error("Priority queue should be empty after all pops")
	}
}

// TestPriorityQueueMinHeap verifies min-heap behavior with reversed
// comparison function.
func TestPriorityQueueMinHeap(t *testing.T) {
	t.Parallel()

	pq := NewPriorityQueue(func(a, b int) bool {
		return a < b
	})

	pq.Push(3)
	pq.Push(1)
	pq.Push(4)
	pq.Push(2)

	expected := []int{1, 2, 3, 4}
	for i, exp := range expected {
		val, ok := pq.Pop()
		if !ok {
			t.Errorf("Pop %d failed", i)
		}
		if val != exp {
			t.Errorf(
				"Expected Pop to return %d at position %d, got %d",
				exp,
				i,
				val,
			)
		}
	}
}

// TestPriorityQueueIterate verifies iteration yields items in priority order
// without modifying the original queue state.
func TestPriorityQueueIterate(t *testing.T) {
	t.Parallel()

	pq := NewPriorityQueue(func(a, b int) bool {
		return a > b
	})

	pq.Push(3)
	pq.Push(1)
	pq.Push(4)
	pq.Push(2)

	expected := []int{4, 3, 2, 1}
	idx := 0
	for val := range pq.Iterate() {
		if val != expected[idx] {
			t.Errorf(
				"Expected %d at index %d, got %d",
				expected[idx],
				idx,
				val,
			)
		}
		idx++
	}

	if idx != 4 {
		t.Errorf("Expected 4 iterations, got %d", idx)
	}

	if pq.Len() != 4 {
		t.Errorf(
			"Iteration should not modify priority queue, length is %d",
			pq.Len(),
		)
	}

	val, _ := pq.Pop()
	if val != 4 {
		t.Errorf(
			"Original priority queue corrupted, expected 4, got %d",
			val,
		)
	}
}

// TestPriorityQueueClear verifies Clear operation empties the priority queue.
func TestPriorityQueueClear(t *testing.T) {
	t.Parallel()

	pq := NewPriorityQueue(func(a, b int) bool {
		return a > b
	})

	pq.Push(1)
	pq.Push(2)
	pq.Push(3)

	pq.Clear()

	if !pq.IsEmpty() {
		t.Error("Priority queue should be empty after Clear")
	}
	if pq.Len() != 0 {
		t.Errorf("Expected length 0 after Clear, got %d", pq.Len())
	}
}

// TestCollectionsWithCapacity verifies pre-allocation with initial capacity
// doesn't affect empty state behavior.
func TestCollectionsWithCapacity(t *testing.T) {
	t.Parallel()

	stack := NewStack[int](10)
	if stack.Len() != 0 {
		t.Error("Stack with capacity should start empty")
	}

	queue := NewQueue[int](10)
	if queue.Len() != 0 {
		t.Error("Queue with capacity should start empty")
	}

	pq := NewPriorityQueue(func(a, b int) bool { return a > b }, 10)
	if pq.Len() != 0 {
		t.Error("PriorityQueue with capacity should start empty")
	}
}

// TestStackWithStrings verifies Stack works correctly with non-int types.
func TestStackWithStrings(t *testing.T) {
	t.Parallel()

	stack := NewStack[string]()
	stack.Push("first")
	stack.Push("second")
	stack.Push("third")

	val, _ := stack.Pop()
	if val != "third" {
		t.Errorf("Expected 'third', got '%s'", val)
	}
	val, _ = stack.Pop()
	if val != "second" {
		t.Errorf("Expected 'second', got '%s'", val)
	}
	val, _ = stack.Pop()
	if val != "first" {
		t.Errorf("Expected 'first', got '%s'", val)
	}
}

// TestEarlyExitFromIteration verifies iterators respect early break and don't
// continue yielding values after consumer stops.
func TestEarlyExitFromIteration(t *testing.T) {
	t.Parallel()

	stack := NewStack[int]()
	for i := 0; i < 10; i++ {
		stack.Push(i)
	}

	count := 0
	for range stack.Iterate() {
		count++
		if count == 3 {
			break
		}
	}
	if count != 3 {
		t.Errorf("Expected 3 iterations, got %d", count)
	}

	queue := NewQueue[int]()
	for i := 0; i < 10; i++ {
		queue.Enqueue(i)
	}

	count = 0
	for range queue.Iterate() {
		count++
		if count == 3 {
			break
		}
	}
	if count != 3 {
		t.Errorf("Expected 3 iterations, got %d", count)
	}

	pq := NewPriorityQueue(func(a, b int) bool { return a > b })
	for i := 0; i < 10; i++ {
		pq.Push(i)
	}

	count = 0
	for range pq.Iterate() {
		count++
		if count == 3 {
			break
		}
	}
	if count != 3 {
		t.Errorf("Expected 3 iterations, got %d", count)
	}
}