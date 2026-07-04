// Package vectorarch implements fixed-width deck archetype vector math.
package vectorarch

import (
	"math"

	"matchpoint/contracts"
)

const (
	VectorDimensionCount   = contracts.VectorDimensionCount
	VectorZeroThreshold    = contracts.VectorZeroThreshold
	VectorCounterThreshold = contracts.VectorCounterThreshold
	VectorSimilarThreshold = contracts.VectorSimilarThreshold
)

const (
	VectorStatusOK            = contracts.VectorStatusOK
	VectorStatusZeroVector    = contracts.VectorStatusZeroVector
	VectorStatusInvalidConfig = contracts.VectorStatusInvalidConfig
	VectorStatusInvalidInput  = contracts.VectorStatusInvalidInput
)

const (
	VectorPairDefault = contracts.VectorPairDefault
	VectorPairCounter = contracts.VectorPairCounter
	VectorPairSimilar = contracts.VectorPairSimilar
)

type VectorStatus = contracts.VectorStatus
type VectorPairClass = contracts.VectorPairClass
type VectorConfig = contracts.VectorConfig
type VectorAnalysis = contracts.VectorAnalysis
type ArchetypeVectorEngine = contracts.ArchetypeVectorEngine

type engine struct {
	config contracts.VectorConfig
}

func productionConfig() contracts.VectorConfig {
	return contracts.VectorConfig{
		ZeroThreshold:    contracts.VectorZeroThreshold,
		CounterThreshold: contracts.VectorCounterThreshold,
		SimilarThreshold: contracts.VectorSimilarThreshold,
	}
}

func newEngine(config contracts.VectorConfig) (*engine, contracts.VectorStatus) {
	config = fillDefaults(config)
	if status := validateConfig(config); status != contracts.VectorStatusOK {
		return nil, status
	}
	return &engine{config: config}, contracts.VectorStatusOK
}

func fillDefaults(config contracts.VectorConfig) contracts.VectorConfig {
	if config.ZeroThreshold == 0 {
		config.ZeroThreshold = contracts.VectorZeroThreshold
	}
	if config.CounterThreshold == 0 {
		config.CounterThreshold = contracts.VectorCounterThreshold
	}
	if config.SimilarThreshold == 0 {
		config.SimilarThreshold = contracts.VectorSimilarThreshold
	}
	return config
}

func validateConfig(config contracts.VectorConfig) contracts.VectorStatus {
	if !finite(config.ZeroThreshold) || config.ZeroThreshold <= 0 ||
		!finite(config.CounterThreshold) || config.CounterThreshold < -1 || config.CounterThreshold > 1 ||
		!finite(config.SimilarThreshold) || config.SimilarThreshold < -1 || config.SimilarThreshold > 1 ||
		config.CounterThreshold >= config.SimilarThreshold {
		return contracts.VectorStatusInvalidConfig
	}
	return contracts.VectorStatusOK
}

func (e *engine) Normalize(raw [8]float32, out *[8]float32) contracts.VectorStatus {
	if e == nil || out == nil {
		return contracts.VectorStatusInvalidInput
	}
	var sum float32
	for i := 0; i < len(raw); i++ {
		v := raw[i]
		if !finite(v) {
			return contracts.VectorStatusInvalidInput
		}
		sum += v * v
	}
	if sum < e.config.ZeroThreshold*e.config.ZeroThreshold {
		return contracts.VectorStatusZeroVector
	}
	inv := float32(1.0 / math.Sqrt(float64(sum)))
	for i := 0; i < len(raw); i++ {
		out[i] = raw[i] * inv
	}
	return contracts.VectorStatusOK
}

func (e *engine) CosineSimilarity(a [8]float32, b [8]float32) float32 {
	_ = e
	var dot float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return clamp(dot, -1, 1)
}

func (e *engine) AnalyzePair(a [8]float32, b [8]float32, out *contracts.VectorAnalysis) contracts.VectorStatus {
	if e == nil || out == nil || !finiteVector(a) || !finiteVector(b) {
		return contracts.VectorStatusInvalidInput
	}
	similarity := e.CosineSimilarity(a, b)
	class := contracts.VectorPairDefault
	if similarity < e.config.CounterThreshold {
		class = contracts.VectorPairCounter
	} else if similarity > e.config.SimilarThreshold {
		class = contracts.VectorPairSimilar
	}
	*out = contracts.VectorAnalysis{
		Similarity: similarity,
		Distance:   1 - similarity,
		Class:      class,
	}
	return contracts.VectorStatusOK
}

func finiteVector(v [8]float32) bool {
	for i := 0; i < len(v); i++ {
		if !finite(v[i]) {
			return false
		}
	}
	return true
}

func finite(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

func clamp(v float32, min float32, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
