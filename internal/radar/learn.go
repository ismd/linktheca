package radar

import (
	"errors"
	"math"
)

// How a topic learns from the user's marks, measured on the matching
// research corpus: docs/superpowers/plans/2026-11-27-radar-topic-feedback.md.
const (
	// learnAlpha is how far ✓ moves the topic towards the marked findings:
	// q' = normalize(q + learnAlpha · mean of their unit match vectors).
	learnAlpha = 1.0

	// cutoffMargin is how far the cutoff sits above a ✗ finding's score and
	// below a ✓ finding's.
	cutoffMargin = 0.005

	// streamMeanSample is how many recent findings the stream's mean match
	// vector is taken over.
	streamMeanSample = 2000
)

// Verdicts a user can give a finding for one topic.
const (
	VerdictRelevant   = "relevant"
	VerdictIrrelevant = "irrelevant"
)

func validVerdict(v string) bool {
	return v == VerdictRelevant || v == VerdictIrrelevant
}

// Example is a finding the user marked for a topic: its match vector and
// whether they said it is about the topic.
type Example struct {
	FindingID int64
	Vec       []float32
	Relevant  bool
}

// TopicModel is what a topic is matched with. A finding with match vector m
// scores m · MatchVec − Offset and matches when that reaches Cutoff.
type TopicModel struct {
	MatchVec []float32
	Offset   float32
	Cutoff   float32
}

// Score returns a finding's score against the model.
func (m TopicModel) Score(findingVec []float32) float32 {
	return float32(dot(findingVec, m.MatchVec)) - m.Offset
}

// LearnTopic builds a topic's model from the embedding q of its description,
// the reference centroid c, the user's threshold and the findings they marked.
//
//   - ✓ findings move the topic towards them: q' = normalize(q + α·mean(m̂)).
//     Moving towards articles raises every article's score, so the shift of
//     the stream's mean score, streamMean · (q' − q), is subtracted as the
//     offset; without it a fixed cutoff lets the stream in. streamMean may be
//     nil when there is no stream yet, and is only needed with ✓ findings.
//   - ✗ findings raise the cutoff just above the highest of them; ✓ findings
//     keep it just below the lowest of them. Neither takes it below the
//     threshold.
//
// With no marks the model is q − c with the threshold as cutoff.
func LearnTopic(q, centroid, streamMean []float32, threshold float32, examples []Example) (TopicModel, error) {
	if len(q) == 0 || len(q) != len(centroid) {
		return TopicModel{}, errors.New("topic and reference embeddings differ in dimensions")
	}

	var relevant, irrelevant [][]float32
	for _, e := range examples {
		if len(e.Vec) != len(q) {
			return TopicModel{}, errors.New("a marked finding's match vector differs in dimensions")
		}
		if e.Relevant {
			relevant = append(relevant, e.Vec)
		} else {
			irrelevant = append(irrelevant, e.Vec)
		}
	}

	moved, offset := moveTowards(q, relevant, streamMean)

	model := TopicModel{MatchVec: make([]float32, len(q)), Offset: offset, Cutoff: threshold}
	for i := range q {
		model.MatchVec[i] = moved[i] - centroid[i]
	}

	for _, v := range irrelevant {
		model.Cutoff = max(model.Cutoff, model.Score(v)+cutoffMargin)
	}

	if len(relevant) > 0 {
		lowest := float32(math.Inf(1))
		for _, v := range relevant {
			lowest = min(lowest, model.Score(v))
		}
		model.Cutoff = min(model.Cutoff, max(threshold, lowest-cutoffMargin))
	}

	return model, nil
}

// moveTowards returns q moved towards the mean direction of the relevant
// vectors and renormalized, with the shift of the stream's mean score it
// causes. Without relevant vectors q is returned as is.
func moveTowards(q []float32, relevant [][]float32, streamMean []float32) ([]float32, float32) {
	if len(relevant) == 0 {
		return q, 0
	}

	sum := make([]float64, len(q))
	for _, v := range relevant {
		norm := math.Sqrt(dot(v, v))
		if norm == 0 {
			continue
		}
		for i, x := range v {
			sum[i] += float64(x) / norm
		}
	}

	moved := make([]float64, len(q))
	var norm float64
	for i := range q {
		moved[i] = float64(q[i]) + learnAlpha*sum[i]/float64(len(relevant))
		norm += moved[i] * moved[i]
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return q, 0
	}

	out := make([]float32, len(q))
	var shift float64
	for i := range q {
		out[i] = float32(moved[i] / norm)
		if len(streamMean) == len(q) {
			shift += float64(streamMean[i]) * (float64(out[i]) - float64(q[i]))
		}
	}

	return out, float32(shift)
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
