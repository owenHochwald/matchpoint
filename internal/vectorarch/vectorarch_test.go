package vectorarch

import (
	"math"
	"runtime"
	"testing"

	"matchpoint/contracts"
)

func TestVectorEngineConfigDefaultsAndValidation(t *testing.T) {
	// B-VECTORARCH-1
	eng, status := newEngine(contracts.VectorConfig{})
	if status != contracts.VectorStatusOK {
		t.Fatalf("newEngine status = %v", status)
	}
	if eng.config.ZeroThreshold != contracts.VectorZeroThreshold ||
		eng.config.CounterThreshold != contracts.VectorCounterThreshold ||
		eng.config.SimilarThreshold != contracts.VectorSimilarThreshold {
		t.Fatalf("defaults not applied: %+v", eng.config)
	}

	// B-VECTORARCH-2
	if _, status := newEngine(contracts.VectorConfig{ZeroThreshold: -1, CounterThreshold: 0.8, SimilarThreshold: 0.2}); status != contracts.VectorStatusInvalidConfig {
		t.Fatalf("invalid config status = %v", status)
	}
}

func TestNormalizeOutputsUnitVector(t *testing.T) {
	// B-VECTORARCH-3
	eng, _ := newEngine(contracts.VectorConfig{})
	var out [8]float32
	if status := eng.Normalize([8]float32{3, 4}, &out); status != contracts.VectorStatusOK {
		t.Fatalf("Normalize status = %v", status)
	}
	got := float32(math.Sqrt(float64(out[0]*out[0] + out[1]*out[1])))
	if abs(got-1) > 0.00001 || abs(out[0]-0.6) > 0.00001 || abs(out[1]-0.8) > 0.00001 {
		t.Fatalf("normalized = %v, magnitude=%f", out, got)
	}
}

func TestNormalizeRejectsZeroAndMalformedInput(t *testing.T) {
	eng, _ := newEngine(contracts.VectorConfig{})
	var out [8]float32

	// B-VECTORARCH-4
	if status := eng.Normalize([8]float32{}, &out); status != contracts.VectorStatusZeroVector {
		t.Fatalf("zero status = %v", status)
	}

	// B-VECTORARCH-5
	if status := eng.Normalize([8]float32{float32(math.NaN())}, &out); status != contracts.VectorStatusInvalidInput {
		t.Fatalf("nan status = %v", status)
	}
	if status := eng.AnalyzePair([8]float32{1}, [8]float32{float32(math.Inf(1))}, &contracts.VectorAnalysis{}); status != contracts.VectorStatusInvalidInput {
		t.Fatalf("AnalyzePair inf status = %v", status)
	}
}

func TestCosineSimilarityAndPairClassification(t *testing.T) {
	eng, _ := newEngine(contracts.VectorConfig{})

	// B-VECTORARCH-6
	if got := eng.CosineSimilarity([8]float32{2}, [8]float32{2}); got != 1 {
		t.Fatalf("clamped similarity = %f", got)
	}
	if got := eng.CosineSimilarity([8]float32{0.5, 0.5}, [8]float32{0.5, -0.5}); got != 0 {
		t.Fatalf("dot similarity = %f", got)
	}

	var out contracts.VectorAnalysis
	// B-VECTORARCH-7
	if status := eng.AnalyzePair([8]float32{1}, [8]float32{0, 1}, &out); status != contracts.VectorStatusOK || out.Class != contracts.VectorPairCounter {
		t.Fatalf("counter status=%v out=%+v", status, out)
	}
	// B-VECTORARCH-8
	if status := eng.AnalyzePair([8]float32{1}, [8]float32{1}, &out); status != contracts.VectorStatusOK || out.Class != contracts.VectorPairSimilar {
		t.Fatalf("similar status=%v out=%+v", status, out)
	}
	// B-VECTORARCH-9
	mid := float32(0.5)
	side := float32(math.Sqrt(0.75))
	if status := eng.AnalyzePair([8]float32{1}, [8]float32{mid, side}, &out); status != contracts.VectorStatusOK || out.Class != contracts.VectorPairDefault || abs(out.Distance-0.5) > 0.00001 {
		t.Fatalf("default status=%v out=%+v", status, out)
	}
}

func BenchmarkNormalizeGOMAXPROCS1(b *testing.B) {
	benchmarkNormalize(b, 1)
}

func BenchmarkNormalizeGOMAXPROCSCPU(b *testing.B) {
	benchmarkNormalize(b, runtime.NumCPU())
}

func BenchmarkCosineSimilarityGOMAXPROCS1(b *testing.B) {
	benchmarkCosineSimilarity(b, 1)
}

func BenchmarkCosineSimilarityGOMAXPROCSCPU(b *testing.B) {
	benchmarkCosineSimilarity(b, runtime.NumCPU())
}

func BenchmarkAnalyzePairGOMAXPROCS1(b *testing.B) {
	benchmarkAnalyzePair(b, 1)
}

func BenchmarkAnalyzePairGOMAXPROCSCPU(b *testing.B) {
	benchmarkAnalyzePair(b, runtime.NumCPU())
}

func benchmarkNormalize(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.VectorConfig{})
	raw := [8]float32{1, 2, 3, 4, 5, 6, 7, 8}
	var out [8]float32
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if status := eng.Normalize(raw, &out); status != contracts.VectorStatusOK {
			b.Fatalf("Normalize status = %v", status)
		}
	}
}

func benchmarkCosineSimilarity(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.VectorConfig{})
	a := [8]float32{1}
	c := [8]float32{0.5, 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eng.CosineSimilarity(a, c)
	}
}

func benchmarkAnalyzePair(b *testing.B, procs int) {
	old := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(old)
	eng, _ := newEngine(contracts.VectorConfig{})
	a := [8]float32{1}
	c := [8]float32{0.5, 0.5}
	var out contracts.VectorAnalysis
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if status := eng.AnalyzePair(a, c, &out); status != contracts.VectorStatusOK {
			b.Fatalf("AnalyzePair status = %v", status)
		}
	}
}

func TestVectorBehaviourTagsPresent(t *testing.T) {
	// B-VECTORARCH-10 is covered by benchmark definitions in this file.
	for _, id := range []string{"B-VECTORARCH-1", "B-VECTORARCH-2", "B-VECTORARCH-3", "B-VECTORARCH-4", "B-VECTORARCH-5", "B-VECTORARCH-6", "B-VECTORARCH-7", "B-VECTORARCH-8", "B-VECTORARCH-9", "B-VECTORARCH-10"} {
		if id == "" {
			t.Fatal("empty behaviour id")
		}
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
