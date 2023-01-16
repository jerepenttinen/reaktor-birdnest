package myredis

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"github.com/go-redis/redis/v9"
	"log"
	"sync/atomic"
	"time"
)

var queueKey = "queue"

type MyRedis[T any] struct {
	done  chan bool
	dirty atomic.Bool
	ctx   context.Context
	rdb   *redis.Client
	ttl   time.Duration
}

func New[T any](opt *redis.Options, ttl time.Duration) *MyRedis[T] {
	ctx := context.Background()
	rdb := redis.NewClient(opt)

	rdb.FlushDB(ctx)

	_, err := rdb.ConfigSet(ctx, "notify-keyspace-events", "KEA").Result()
	if err != nil {
		log.Panicf("unable to set keyspace events %v", err)
	}

	p := rdb.PSubscribe(ctx, "__keyevent@0__:expired")
	result := &MyRedis[T]{
		rdb:  rdb,
		ctx:  ctx,
		done: make(chan bool, 1),
		ttl:  ttl,
	}

	go func(stop <-chan bool) {
		for {
			select {
			case <-stop:
				return
			default:
				msg, err := p.ReceiveMessage(ctx)
				if err != nil {
					fmt.Printf("error %v", err)
					return
				}
				result.rdb.ZRem(result.ctx, queueKey, msg.String())
				result.dirty.Store(true)
			}

		}
	}(result.done)

	return result
}

func (m *MyRedis[T]) Get(id string) (T, bool) {
	var result T
	bs, err := m.rdb.Get(m.ctx, id).Bytes()
	if err != nil {
		return result, false
	}

	r := bytes.NewReader(bs)
	err = gob.NewDecoder(r).Decode(&result)
	if err != nil {
		return result, false
	}

	return result, true
}

func (m *MyRedis[T]) Upsert(id string, data T) {
	buf := bytes.Buffer{}
	gob.NewEncoder(&buf).Encode(data)

	m.rdb.Set(m.ctx, id, buf.Bytes(), m.ttl)
	m.rdb.ZAdd(m.ctx, queueKey, redis.Z{
		Member: id,
		Score:  float64(time.Now().UTC().Unix()),
	})
	m.dirty.Store(true)
}

func (m *MyRedis[T]) Destroy() {
	m.rdb.Close()
}

func (m *MyRedis[T]) AsSlice() []T {
	queue := m.rdb.ZRevRange(m.ctx, queueKey, 0, -1).Val()
	violationBuffers := m.rdb.MGet(m.ctx, queue...).Val()
	result := make([]T, 0, len(violationBuffers))
	for _, violationBuffer := range violationBuffers {
		if violationBuffer == nil {
			continue
		}
		var decoded T
		reader := bytes.NewReader([]byte(violationBuffer.(string)))
		gob.NewDecoder(reader).Decode(&decoded)
		result = append(result, decoded)
	}

	return result
}

func (m *MyRedis[T]) HasChanges() bool {
	return m.dirty.Swap(false)
}
