package collection

import (
	"container/list"
	"errors"
)

type Queue struct {
	list list.List
}

/*
* Returns an initialized queue
 */
func NewQueue() Queue {
	return Queue{
		list: *list.New(),
	}
}

func (q *Queue) IsEmpty() bool {
	return q.list.Len() > 0
}

func (q *Queue) Enqueue(v interface{}) error {
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

	// Get the first element in the list
	ele := q.list.Front()
	return q.list.Remove(ele) // pop it
}
