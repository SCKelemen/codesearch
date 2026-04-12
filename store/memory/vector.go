package memory

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/SCKelemen/codesearch/store"
)

// VectorStoreOption configures the in-memory HNSW store.
type VectorStoreOption func(*VectorStore)

// WithM sets the maximum number of graph neighbors above layer zero.
func WithM(m int) VectorStoreOption {
	return func(s *VectorStore) {
		if m > 1 {
			s.m = m
		}
	}
}

// WithEFConstruction sets the insertion-time candidate breadth.
func WithEFConstruction(ef int) VectorStoreOption {
	return func(s *VectorStore) {
		if ef > 0 {
			s.efConstruction = ef
		}
	}
}

// WithEFSearch sets the default query-time candidate breadth.
func WithEFSearch(ef int) VectorStoreOption {
	return func(s *VectorStore) {
		if ef > 0 {
			s.efSearch = ef
		}
	}
}

// WithRandomSeed makes level assignment deterministic.
func WithRandomSeed(seed int64) VectorStoreOption {
	return func(s *VectorStore) {
		s.rand = rand.New(rand.NewSource(seed))
	}
}

type vectorNode struct {
	internalID string
	vector     store.StoredVector
	normalized []float32
	level      int
	deleted    bool
	neighbors  map[store.DistanceMetric][][]string
}

type queryVector struct {
	values     []float32
	normalized []float32
}

type hnswGraph struct {
	entrypoint string
	maxLevel   int
}

type candidate struct {
	id       string
	distance float32
}

type minCandidateHeap []candidate

func (h minCandidateHeap) Len() int { return len(h) }
func (h minCandidateHeap) Less(i, j int) bool {
	if h[i].distance != h[j].distance {
		return h[i].distance < h[j].distance
	}
	return h[i].id < h[j].id
}
func (h minCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *minCandidateHeap) Push(x any)   { *h = append(*h, x.(candidate)) }
func (h *minCandidateHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	*h = old[:len(old)-1]
	return item
}

type maxCandidateHeap []candidate

func (h maxCandidateHeap) Len() int { return len(h) }
func (h maxCandidateHeap) Less(i, j int) bool {
	if h[i].distance != h[j].distance {
		return h[i].distance > h[j].distance
	}
	return h[i].id > h[j].id
}
func (h maxCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *maxCandidateHeap) Push(x any)   { *h = append(*h, x.(candidate)) }
func (h *maxCandidateHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	*h = old[:len(old)-1]
	return item
}

// VectorStore is an in-memory HNSW-backed implementation of store.VectorStore.
type VectorStore struct {
	mu             sync.RWMutex
	randMu         sync.Mutex
	sequence       atomic.Uint64
	active         map[string]string
	nodes          map[string]*vectorNode
	graphs         map[store.DistanceMetric]*hnswGraph
	m              int
	efConstruction int
	efSearch       int
	rand           *rand.Rand
}

// NewVectorStore creates an empty HNSW-backed vector store.
func NewVectorStore(opts ...VectorStoreOption) *VectorStore {
	s := &VectorStore{
		active: make(map[string]string),
		nodes:  make(map[string]*vectorNode),
		graphs: map[store.DistanceMetric]*hnswGraph{
			store.DistanceMetricCosine:     {},
			store.DistanceMetricEuclidean:  {},
			store.DistanceMetricDotProduct: {},
		},
		m:              16,
		efConstruction: 200,
		efSearch:       64,
		rand:           rand.New(rand.NewSource(1)),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *VectorStore) Put(_ context.Context, vector store.StoredVector) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if previousID, ok := s.active[vector.ID]; ok {
		if previous := s.nodes[previousID]; previous != nil {
			previous.deleted = true
		}
	}

	node := &vectorNode{
		internalID: s.nextInternalID(vector.ID),
		vector:     cloneVector(vector),
		normalized: normalizeVector(vector.Values),
		level:      s.randomLevel(),
		neighbors:  make(map[store.DistanceMetric][][]string, len(s.graphs)),
	}
	for metric := range s.graphs {
		node.neighbors[metric] = make([][]string, node.level+1)
	}

	s.nodes[node.internalID] = node
	s.active[vector.ID] = node.internalID
	for metric, graph := range s.graphs {
		s.insertNode(graph, metric, node)
	}
	return nil
}

func (s *VectorStore) Lookup(_ context.Context, id string, opts ...store.LookupOption) (*store.StoredVector, error) {
	options := store.ResolveLookupOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	internalID, ok := s.active[id]
	if !ok {
		return nil, nil
	}
	node := s.nodes[internalID]
	if node == nil || node.deleted || !matchesVectorFilter(node.vector, options.Filter) {
		return nil, nil
	}
	clone := cloneVector(node.vector)
	return &clone, nil
}

func (s *VectorStore) List(_ context.Context, opts ...store.ListOption) ([]store.StoredVector, string, error) {
	options := store.ResolveListOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]store.StoredVector, 0, len(s.active))
	for _, vectorID := range sortedStringKeys(s.active) {
		node := s.nodes[s.active[vectorID]]
		if node == nil || node.deleted || !matchesVectorFilter(node.vector, options.Filter) {
			continue
		}
		items = append(items, cloneVector(node.vector))
	}
	return applyPage(items, options.Cursor, options.Limit)
}

func (s *VectorStore) Search(_ context.Context, query []float32, k int, metric store.DistanceMetric, opts ...store.SearchOption) ([]store.VectorResult, error) {
	if k <= 0 {
		return nil, nil
	}
	options := store.ResolveSearchOptions(opts...)
	offset, err := parseCursor(options.Cursor)
	if err != nil {
		return nil, err
	}
	limit := k
	if options.Limit > 0 && options.Limit < limit {
		limit = options.Limit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	graph, ok := s.graphs[metric]
	if !ok {
		return nil, fmt.Errorf("unsupported distance metric %q", metric)
	}
	if graph.entrypoint == "" {
		return nil, nil
	}

	queryView := makeQueryVector(query)
	entrypoint := graph.entrypoint
	for level := graph.maxLevel; level > 0; level-- {
		entrypoint = s.greedySearchAtLayer(metric, queryView, entrypoint, level)
	}

	searchBreadth := maxInt(s.efSearch, k+offset)
	candidates := s.searchLayer(metric, queryView, []string{entrypoint}, searchBreadth, 0)
	results := s.rankCandidates(metric, queryView, candidates, options.Filter)
	if len(results) > k+offset {
		results = results[:k+offset]
	}
	page, err := applySearchPage(results, options.Cursor, limit)
	if err != nil {
		return nil, err
	}
	for index := range page {
		page[index].Rank = offset + index + 1
	}
	return page, nil
}

func (s *VectorStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	internalID, ok := s.active[id]
	if !ok {
		return nil
	}
	if node := s.nodes[internalID]; node != nil {
		node.deleted = true
	}
	delete(s.active, id)
	return nil
}

func (s *VectorStore) insertNode(graph *hnswGraph, metric store.DistanceMetric, node *vectorNode) {
	if graph.entrypoint == "" {
		graph.entrypoint = node.internalID
		graph.maxLevel = node.level
		return
	}

	query := queryVector{values: node.vector.Values, normalized: node.normalized}
	entrypoint := graph.entrypoint
	for level := graph.maxLevel; level > node.level; level-- {
		entrypoint = s.greedySearchAtLayer(metric, query, entrypoint, level)
	}
	for level := minInt(node.level, graph.maxLevel); level >= 0; level-- {
		candidates := s.searchLayer(metric, query, []string{entrypoint}, s.efConstruction, level)
		neighbors := s.selectNeighbors(candidates, node.internalID, s.maxConnections(level))
		for _, neighborID := range neighbors {
			s.connectBidirectional(metric, node.internalID, neighborID, level)
		}
		if len(candidates) != 0 {
			entrypoint = candidates[0].id
		}
	}
	if node.level > graph.maxLevel {
		graph.entrypoint = node.internalID
		graph.maxLevel = node.level
	}
}

func (s *VectorStore) connectBidirectional(metric store.DistanceMetric, aID string, bID string, level int) {
	a := s.nodes[aID]
	b := s.nodes[bID]
	if a == nil || b == nil || level >= len(a.neighbors[metric]) || level >= len(b.neighbors[metric]) {
		return
	}
	a.neighbors[metric][level] = appendUnique(a.neighbors[metric][level], bID)
	b.neighbors[metric][level] = appendUnique(b.neighbors[metric][level], aID)
	a.neighbors[metric][level] = s.pruneNeighbors(metric, a, a.neighbors[metric][level], s.maxConnections(level))
	b.neighbors[metric][level] = s.pruneNeighbors(metric, b, b.neighbors[metric][level], s.maxConnections(level))
}

func (s *VectorStore) pruneNeighbors(metric store.DistanceMetric, node *vectorNode, neighborIDs []string, max int) []string {
	if len(neighborIDs) <= max {
		return neighborIDs
	}
	scored := make([]candidate, 0, len(neighborIDs))
	seen := make(map[string]struct{}, len(neighborIDs))
	for _, neighborID := range neighborIDs {
		if _, ok := seen[neighborID]; ok {
			continue
		}
		seen[neighborID] = struct{}{}
		neighbor := s.nodes[neighborID]
		if neighbor == nil {
			continue
		}
		scored = append(scored, candidate{id: neighborID, distance: s.distance(metric, node, neighbor)})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].distance != scored[j].distance {
			return scored[i].distance < scored[j].distance
		}
		return scored[i].id < scored[j].id
	})
	out := make([]string, 0, minInt(max, len(scored)))
	for _, item := range scored[:minInt(max, len(scored))] {
		out = append(out, item.id)
	}
	return out
}

func (s *VectorStore) selectNeighbors(candidates []candidate, selfID string, max int) []string {
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].id < candidates[j].id
	})
	out := make([]string, 0, minInt(max, len(candidates)))
	seen := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		if item.id == selfID {
			continue
		}
		if _, ok := seen[item.id]; ok {
			continue
		}
		seen[item.id] = struct{}{}
		out = append(out, item.id)
		if len(out) == max {
			break
		}
	}
	return out
}

func (s *VectorStore) greedySearchAtLayer(metric store.DistanceMetric, query queryVector, entrypoint string, level int) string {
	bestID := entrypoint
	bestNode := s.nodes[bestID]
	if bestNode == nil {
		return entrypoint
	}
	bestDistance := s.queryDistance(metric, query, bestNode)
	for {
		improved := false
		for _, neighborID := range s.neighborsAt(bestNode, metric, level) {
			neighbor := s.nodes[neighborID]
			if neighbor == nil {
				continue
			}
			distance := s.queryDistance(metric, query, neighbor)
			if distance < bestDistance {
				bestID = neighborID
				bestNode = neighbor
				bestDistance = distance
				improved = true
			}
		}
		if !improved {
			return bestID
		}
	}
}

func (s *VectorStore) searchLayer(metric store.DistanceMetric, query queryVector, entrypoints []string, ef int, level int) []candidate {
	if ef <= 0 {
		ef = 1
	}
	visited := make(map[string]struct{}, len(entrypoints))
	candidates := &minCandidateHeap{}
	best := &maxCandidateHeap{}
	heap.Init(candidates)
	heap.Init(best)

	for _, entryID := range entrypoints {
		node := s.nodes[entryID]
		if node == nil {
			continue
		}
		item := candidate{id: entryID, distance: s.queryDistance(metric, query, node)}
		heap.Push(candidates, item)
		heap.Push(best, item)
		visited[entryID] = struct{}{}
	}
	if best.Len() == 0 {
		return nil
	}

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(candidate)
		worst := (*best)[0]
		if best.Len() >= ef && current.distance > worst.distance {
			break
		}
		currentNode := s.nodes[current.id]
		if currentNode == nil {
			continue
		}
		for _, neighborID := range s.neighborsAt(currentNode, metric, level) {
			if _, ok := visited[neighborID]; ok {
				continue
			}
			visited[neighborID] = struct{}{}
			neighbor := s.nodes[neighborID]
			if neighbor == nil {
				continue
			}
			distance := s.queryDistance(metric, query, neighbor)
			if best.Len() < ef || distance < (*best)[0].distance {
				item := candidate{id: neighborID, distance: distance}
				heap.Push(candidates, item)
				heap.Push(best, item)
				if best.Len() > ef {
					heap.Pop(best)
				}
			}
		}
	}

	out := make([]candidate, best.Len())
	for index := len(out) - 1; index >= 0; index-- {
		out[index] = heap.Pop(best).(candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].distance != out[j].distance {
			return out[i].distance < out[j].distance
		}
		return out[i].id < out[j].id
	})
	return out
}

func (s *VectorStore) rankCandidates(metric store.DistanceMetric, query queryVector, candidates []candidate, filter store.Filter) []store.VectorResult {
	results := make([]store.VectorResult, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		node := s.nodes[item.id]
		if node == nil || node.deleted {
			continue
		}
		activeID, ok := s.active[node.vector.ID]
		if !ok || activeID != node.internalID {
			continue
		}
		if _, ok := seen[node.vector.ID]; ok {
			continue
		}
		if !matchesVectorFilter(node.vector, filter) {
			continue
		}
		seen[node.vector.ID] = struct{}{}
		distance := s.queryDistance(metric, query, node)
		results = append(results, store.VectorResult{
			Vector:   cloneVector(node.vector),
			Distance: distance,
			Score:    scoreFromDistance(metric, distance),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Distance != results[j].Distance {
			return results[i].Distance < results[j].Distance
		}
		return results[i].Vector.ID < results[j].Vector.ID
	})
	return results
}

func (s *VectorStore) neighborsAt(node *vectorNode, metric store.DistanceMetric, level int) []string {
	layers := node.neighbors[metric]
	if level < 0 || level >= len(layers) {
		return nil
	}
	return layers[level]
}

func (s *VectorStore) nextInternalID(vectorID string) string {
	value := s.sequence.Add(1)
	return vectorID + "#" + strconv.FormatUint(value, 10)
}

func (s *VectorStore) randomLevel() int {
	s.randMu.Lock()
	defer s.randMu.Unlock()

	level := 0
	for s.rand.Float64() < 1.0/math.E {
		level++
	}
	return level
}

func (s *VectorStore) maxConnections(level int) int {
	if level == 0 {
		return s.m * 2
	}
	return s.m
}

func appendUnique(values []string, target string) []string {
	for _, value := range values {
		if value == target {
			return values
		}
	}
	return append(values, target)
}

func makeQueryVector(values []float32) queryVector {
	return queryVector{values: values, normalized: normalizeVector(values)}
}

func (s *VectorStore) distance(metric store.DistanceMetric, a *vectorNode, b *vectorNode) float32 {
	switch metric {
	case store.DistanceMetricDotProduct:
		return -dotProduct(a.vector.Values, b.vector.Values)
	case store.DistanceMetricEuclidean:
		return euclideanDistance(a.vector.Values, b.vector.Values)
	case store.DistanceMetricCosine:
		fallthrough
	default:
		return 1 - dotProduct(a.normalized, b.normalized)
	}
}

func (s *VectorStore) queryDistance(metric store.DistanceMetric, query queryVector, node *vectorNode) float32 {
	switch metric {
	case store.DistanceMetricDotProduct:
		return -dotProduct(query.values, node.vector.Values)
	case store.DistanceMetricEuclidean:
		return euclideanDistance(query.values, node.vector.Values)
	case store.DistanceMetricCosine:
		fallthrough
	default:
		return 1 - dotProduct(query.normalized, node.normalized)
	}
}

func scoreFromDistance(metric store.DistanceMetric, distance float32) float32 {
	switch metric {
	case store.DistanceMetricDotProduct:
		return -distance
	case store.DistanceMetricEuclidean:
		return 1 / (1 + distance)
	case store.DistanceMetricCosine:
		fallthrough
	default:
		return 1 - distance
	}
}

func matchesVectorFilter(vector store.StoredVector, filter store.Filter) bool {
	if filter.RepositoryID != "" && vector.RepositoryID != filter.RepositoryID {
		return false
	}
	if filter.Branch != "" && vector.Branch != filter.Branch {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(vector.Path, filter.PathPrefix) {
		return false
	}
	if filter.DocumentID != "" && vector.DocumentID != filter.DocumentID {
		return false
	}
	return matchesMetadata(vector.Metadata, filter.Metadata)
}

var _ store.VectorStore = (*VectorStore)(nil)
