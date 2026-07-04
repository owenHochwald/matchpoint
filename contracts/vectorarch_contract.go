// Package contracts defines the Planner-owned public contract for MatchPoint
// modules. This file is intentionally declarative: no implementation logic
// belongs here.
package contracts

// VectorDimensionCount is the fixed deck-archetype vector width.
const VectorDimensionCount = 8

// VectorZeroThreshold is the minimum L2 magnitude accepted for normalization.
const VectorZeroThreshold float32 = 0.000001

// VectorCounterThreshold identifies structural counter archetypes.
const VectorCounterThreshold float32 = EOMMCounterSimilarityThreshold

// VectorSimilarThreshold identifies stylistically similar archetypes.
const VectorSimilarThreshold float32 = EOMMSimilarSimilarityThreshold

// VectorStatus is the stable non-allocating status taxonomy for vectorarch.
type VectorStatus uint8

const (
	// VectorStatusOK means the operation completed successfully.
	VectorStatusOK VectorStatus = 0
	// VectorStatusZeroVector means the magnitude was below VectorZeroThreshold.
	VectorStatusZeroVector VectorStatus = 1
	// VectorStatusInvalidConfig means thresholds are out of range or reversed.
	VectorStatusInvalidConfig VectorStatus = 2
	// VectorStatusInvalidInput means an input contains NaN or infinity.
	VectorStatusInvalidInput VectorStatus = 3
)

// VectorPairClass is the counter/similar/default classification for a pair.
type VectorPairClass uint8

const (
	// VectorPairDefault means neither threshold matched.
	VectorPairDefault VectorPairClass = 0
	// VectorPairCounter means similarity is below the counter threshold.
	VectorPairCounter VectorPairClass = 1
	// VectorPairSimilar means similarity is above the similar threshold.
	VectorPairSimilar VectorPairClass = 2
)

// VectorConfig defines immutable vector scoring thresholds.
type VectorConfig struct {
	// ZeroThreshold defaults to VectorZeroThreshold and must be positive.
	ZeroThreshold float32
	// CounterThreshold defaults to VectorCounterThreshold and must be in [-1, 1].
	CounterThreshold float32
	// SimilarThreshold defaults to VectorSimilarThreshold and must be in [-1, 1].
	SimilarThreshold float32
}

// VectorAnalysis describes one pairwise similarity calculation.
type VectorAnalysis struct {
	// Similarity is dot(a,b), clamped to [-1, 1].
	Similarity float32
	// Distance is 1-Similarity.
	Distance float32
	// Class is counter, similar, or default.
	Class VectorPairClass
}

// ArchetypeVectorEngine owns normalization and cosine-similarity math.
type ArchetypeVectorEngine interface {
	// Normalize L2-normalizes raw into out.
	Normalize(raw [8]float32, out *[8]float32) VectorStatus
	// CosineSimilarity returns dot(a,b) for already-normalized 8-dimensional vectors.
	CosineSimilarity(a [8]float32, b [8]float32) float32
	// AnalyzePair fills similarity, distance, and threshold classification.
	AnalyzePair(a [8]float32, b [8]float32, out *VectorAnalysis) VectorStatus
}
