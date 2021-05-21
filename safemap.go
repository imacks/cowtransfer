package cowtransfer

import (
	"sync"
)

type int64map struct {
	hashmap map[int64]*uploadBlockResult
	mutex sync.RWMutex 
}

type uploadBlockResult struct {
	token string
	err error
	size int
}

func (sm *int64map) Load(key int64) (string, error, bool) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.hashmap == nil {
		sm.hashmap = map[int64]*uploadBlockResult{}
	}

	result, ok := sm.hashmap[key]
	if !ok {
		return "", nil, false
	}
	return result.token, result.err, ok
}

func (sm *int64map) Store(key int64, token string, size int) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.hashmap == nil {
		sm.hashmap = map[int64]*uploadBlockResult{}
	}

	sm.hashmap[key] = &uploadBlockResult{
		token: token,
		err: nil,
		size: size,
	}
}

func (sm *int64map) StoreError(key int64, err error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.hashmap == nil {
		sm.hashmap = map[int64]*uploadBlockResult{}
	}

	sm.hashmap[key] = &uploadBlockResult{
		err: err,
	}
}

func (sm *int64map) Size() (int64, int64) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.hashmap == nil {
		sm.hashmap = map[int64]*uploadBlockResult{}
	}
	
	blocksDone := int64(0)
	sizeDone := int64(0)
	for _, v := range sm.hashmap {
		blocksDone ++
		sizeDone += int64(v.size)
	}

	return blocksDone, sizeDone
}