package datastore

import (
	"container/list"
	"sync"
	"time"
)

type ElementWithID[T any] struct {
	id      string
	touched time.Time
	data    T
}

type DataStore[T any] struct {
	registry map[string]*list.Element
	queue    *list.List
	mut      *sync.RWMutex
	dirty    bool
	ttl      time.Duration
	destroy  chan bool
}

func New[T any](ttl time.Duration) *DataStore[T] {
	result := &DataStore[T]{
		registry: make(map[string]*list.Element),
		queue:    list.New(),
		mut:      &sync.RWMutex{},
		destroy:  make(chan bool, 1),
		ttl:      ttl,
	}
	go result.expire()

	return result
}

func (d *DataStore[T]) expire() {
	ticker := time.NewTicker(time.Millisecond)
	for {
		select {
		case <-d.destroy:
			ticker.Stop()
			break
		case <-ticker.C:
			now := time.Now().UTC()
			d.mut.Lock()
			for back := d.queue.Back(); back != nil; back = d.queue.Back() {
				underlying := back.Value.(*ElementWithID[T])
				if now.Sub(underlying.touched) > d.ttl {
					delete(d.registry, underlying.id)
					d.queue.Remove(back)
					d.dirty = true
				} else {
					break
				}
			}
			d.mut.Unlock()
		default:
			continue
		}
	}
}

func (d *DataStore[T]) Get(id string) (T, bool) {
	d.mut.RLock()
	defer d.mut.RUnlock()
	element, ok := d.registry[id]
	if !ok {
		return *new(T), false
	}

	return element.Value.(*ElementWithID[T]).data, true
}

func (d *DataStore[T]) Upsert(id string, data T) {
	d.mut.Lock()
	defer d.mut.Unlock()

	d.dirty = true
	now := time.Now().UTC()
	if element, ok := d.registry[id]; ok {
		e := element.Value.(*ElementWithID[T])
		e.data = data
		e.touched = now
		d.queue.MoveToFront(element)
	} else {
		element := d.queue.PushFront(&ElementWithID[T]{
			id:      id,
			data:    data,
			touched: now,
		})
		d.registry[id] = element
	}
}

func (d *DataStore[T]) Destroy() {
	d.destroy <- true
}

func (d *DataStore[T]) AsSlice() []T {
	d.mut.RLock()
	defer d.mut.RUnlock()

	result := make([]T, 0, d.queue.Len())
	for element := d.queue.Front(); element != nil; element = element.Next() {
		result = append(result, element.Value.(*ElementWithID[T]).data)
	}
	return result
}

func (d *DataStore[T]) HasChanges() bool {
	defer func() {
		d.dirty = false
	}()
	return d.dirty
}
