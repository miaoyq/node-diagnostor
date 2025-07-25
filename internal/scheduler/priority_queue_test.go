package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriorityQueueFunctionality(t *testing.T) {
	pq := NewPriorityQueue()

	// Test high priority task executes first
	highPriorityTask := &Task{Priority: 10}
	lowPriorityTask := &Task{Priority: 1}

	pq.Push(lowPriorityTask)
	pq.Push(highPriorityTask)

	assert.Equal(t, highPriorityTask, pq.Pop(), "High priority task should execute first")
	assert.Equal(t, lowPriorityTask, pq.Pop(), "Then low priority task")

	// Test equal priority tasks (FIFO)
	task1 := &Task{Priority: 5}
	task2 := &Task{Priority: 5}

	pq.Push(task1)
	pq.Push(task2)

	assert.Equal(t, task1, pq.Pop(), "First added task should execute first when priorities are equal")
	assert.Equal(t, task2, pq.Pop(), "Second added task should execute next when priorities are equal")

	// Test empty queue
	assert.Nil(t, pq.Pop(), "Should return nil when queue is empty")
}
