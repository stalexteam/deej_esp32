package deej

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/thoas/go-funk"
)

type switchMap struct {
	m    map[int][]string
	lock sync.Locker
}

func newSwitchMap() *switchMap {
	return &switchMap{
		m:    make(map[int][]string),
		lock: &sync.Mutex{},
	}
}

func switchMapFromConfigs(userMapping map[string][]string, internalMapping map[string][]string) *switchMap {
	resultMap := newSwitchMap()

	for switchIdxString, targets := range userMapping {
		switchIdx, _ := strconv.Atoi(switchIdxString)

		resultMap.set(switchIdx, funk.FilterString(targets, func(s string) bool {
			return s != ""
		}))
	}

	for switchIdxString, targets := range internalMapping {
		switchIdx, _ := strconv.Atoi(switchIdxString)

		existingTargets, ok := resultMap.get(switchIdx)
		if !ok {
			existingTargets = []string{}
		}

		filteredTargets := funk.FilterString(targets, func(s string) bool {
			return (!funk.ContainsString(existingTargets, s)) && s != ""
		})

		existingTargets = append(existingTargets, filteredTargets...)
		resultMap.set(switchIdx, existingTargets)
	}

	return resultMap
}

func (m *switchMap) get(key int) ([]string, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	value, ok := m.m[key]
	return value, ok
}

func (m *switchMap) set(key int, value []string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.m[key] = value
}

func (m *switchMap) String() string {
	m.lock.Lock()
	defer m.lock.Unlock()

	switchCount := 0
	targetCount := 0

	for _, value := range m.m {
		switchCount++
		targetCount += len(value)
	}

	return fmt.Sprintf("<%d switches mapped to %d targets>", switchCount, targetCount)
}
