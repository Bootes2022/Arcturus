package router

import (
	"forwarding/scheduling_algorithms/k_shortest"
	"sync/atomic"
)

type WeightedRoundRobin struct {
	paths       []k_shortest.PathWithIP
	cumulative  []int
	totalWeight int
	current     uint32
}

func NewWeightedRoundRobin(paths []k_shortest.PathWithIP) *WeightedRoundRobin {

	cumulative := make([]int, len(paths))
	total := 0
	for i, p := range paths {
		total += p.Weight
		cumulative[i] = total
	}
	return &WeightedRoundRobin{
		paths:       paths,
		cumulative:  cumulative,
		totalWeight: total,
		current:     0,
	}
}

func (w *WeightedRoundRobin) Next() k_shortest.PathWithIP {
	if w.totalWeight == 0 || len(w.paths) == 0 {
		return k_shortest.PathWithIP{} //
	}

	n := atomic.AddUint32(&w.current, 1) - 1
	mod := int(n) % w.totalWeight

	for i, c := range w.cumulative {
		if mod < c {
			return w.paths[i]
		}
	}
	return k_shortest.PathWithIP{}
}
