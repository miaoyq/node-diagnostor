package scheduler

import (
	"sync"
)

// PriorityQueue implements a priority queue for tasks
type PriorityQueue struct {
	items []*Task
	mu    sync.RWMutex
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		items: make([]*Task, 0),
	}
}

// Push adds a task to the priority queue
func (pq *PriorityQueue) Push(task *Task) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Insert task maintaining priority order (higher priority first)
	inserted := false
	for i, item := range pq.items {
		if task.Priority > item.Priority {
			pq.items = append(pq.items[:i], append([]*Task{task}, pq.items[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		pq.items = append(pq.items, task)
	}
}

// Pop removes and returns the highest priority task
func (pq *PriorityQueue) Pop() *Task {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.items) == 0 {
		return nil
	}

	task := pq.items[0]
	pq.items = pq.items[1:]
	return task
}

// Len returns the number of tasks in the queue
func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}
