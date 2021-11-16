package collection

import (
	"container/list"
	"errors"
	"sync"
)

type Queue struct {
	lock sync.Mutex
	list list.List
}

/*
* Returns an initialized queue
 */
func NewQueue() Queue {
	return Queue{
		lock: sync.Mutex{},
		list: *list.New(),
	}
}

func (q *Queue) IsEmpty() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.list.Len() == 0
}

func (q *Queue) Enqueue(v interface{}) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	if v == nil {
		return errors.New("Input is nil!")
	}

	q.list.PushBack(v)
	return nil
}

func (q *Queue) Dequeue() interface{} {
	if q.IsEmpty() {
		return nil
	}

	q.lock.Lock()
	defer q.lock.Unlock()

	// Get the first element in the list
	ele := q.list.Front()
	return q.list.Remove(ele) // pop it
}
