package time

import (
	"strconv"
	"sync"

	math "github.com/ap/dme2/internal/math"
)

type LamportTimestamp struct {
	time int32
	lock sync.Mutex
}

func GetLamportTimeStamp() LamportTimestamp {

	return LamportTimestamp{time: 0, lock: sync.Mutex{}}
}

/*
Synchronizes the two timestamps so that the logical timestamp is updated.
*/
func (v *LamportTimestamp) Sync(foreignTimestamp int32) {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.time = math.Max(foreignTimestamp, v.time)
}

func (v *LamportTimestamp) GetDisplayableContent() string {
	return strconv.Itoa(int(v.time))
}

func (v *LamportTimestamp) Increment() {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.time += 1
}