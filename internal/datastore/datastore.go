package datastore

import (
	"container/list"
	"sync"
)

type ElementWithID[T any] struct {
	id   string
	data *T
}

type DataStore[T any] struct {
	registry map[string]*list.Element
	queue    *list.List
	mut      *sync.RWMutex
	dirty    bool
}

func New[T any]() *DataStore[T] {
	return &DataStore[T]{
		registry: make(map[string]*list.Element),
		queue:    list.New(),
		mut:      &sync.RWMutex{},
	}
}

func (d *DataStore[T]) Get(id string) (*T, bool) {
	d.mut.RLock()
	defer d.mut.RUnlock()
	element, ok := d.registry[id]
	if !ok {
		return nil, false
	}

	return element.Value.(ElementWithID[T]).data, true
}

func (d *DataStore[T]) Upsert(id string, data *T) {
	d.mut.Lock()
	defer d.mut.Unlock()

	d.dirty = true
	if element, ok := d.registry[id]; ok {
		e := element.Value.(ElementWithID[T])
		e.data = data
		d.queue.MoveToFront(element)
	} else {
		element := d.queue.PushFront(ElementWithID[T]{
			id:   id,
			data: data,
		})
		d.registry[id] = element
	}
}

func (d *DataStore[T]) DeleteOldestWhile(cond func(T) bool) {
	d.mut.Lock()
	defer d.mut.Unlock()

	for back := d.queue.Back(); back != nil; back = d.queue.Back() {
		underlying := back.Value.(ElementWithID[T])
		if cond(*underlying.data) {
			delete(d.registry, underlying.id)
			d.queue.Remove(back)
			d.dirty = true
		} else {
			return
		}
	}
}

func (d *DataStore[T]) AsSlice() []T {
	d.mut.RLock()
	defer d.mut.RUnlock()

	result := make([]T, 0, d.queue.Len())
	for element := d.queue.Front(); element != nil; element = element.Next() {
		result = append(result, *element.Value.(ElementWithID[T]).data)
	}
	return result
}

func (d *DataStore[T]) HasChanges() bool {
	defer func() {
		d.dirty = false
	}()
	return d.dirty
}
