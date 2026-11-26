package radar

import (
	"context"
	"fmt"
	"sync"

	"github.com/ismd/linktheca/internal/core/embeddings"
)

// referenceTopics are topics people follow on a news radar, spread over many
// fields. Their mean embedding stands for "a topic in general": a finding that
// is close to it is close to every topic, and subtracting it from each topic
// discounts such findings. Changing the list changes every score, so topics
// need their match vectors recomputed afterwards (cmd/reembed-topics).
//
// The list is the one the matching research was measured with, see
// docs/superpowers/specs/2026-11-25-radar-topic-matching-research.md.
var referenceTopics = []string{
	"Formula 1 racing results", "Climate change and extreme weather", "Startup funding rounds",
	"Bitcoin price", "Space telescopes and astronomy", "Chess", "Electric cars and Tesla",
	"Smartphone reviews", "Video game releases", "Machine learning research papers",
	"Cooking recipes", "Interest rates and the Federal Reserve", "Housing market",
	"Open source licensing", "Database performance tuning", "Web browser updates",
	"Cybersecurity breaches and ransomware", "Privacy regulation and GDPR",
	"Mechanical keyboards", "Photography and cameras", "Music streaming",
	"Movies and box office", "Football transfers", "Olympic games", "Archaeology discoveries",
	"Quantum computing", "Semiconductor manufacturing and TSMC", "Programming language design",
	"Functional programming", "Compilers and interpreters", "Operating systems",
	"Cloud computing pricing", "Email and spam", "Typography and fonts", "Urban planning",
	"Public transport", "Aviation and airlines", "Shipping and logistics",
	"Agriculture technology", "Biology and genetics", "Neuroscience", "Mental health",
	"Sleep research", "Nutrition science", "Education and universities",
	"Job market for software engineers", "Layoffs at tech companies",
	"Antitrust cases against big tech", "Social media moderation", "Wikipedia",
	"Retro computing", "Amateur radio", "3D printing", "Home networking and routers",
	"Self-hosting software", "Personal finance and investing",
	"Productivity and note-taking apps", "Mathematics and proofs", "Physics discoveries",
	"Batteries and energy storage", "Solar power", "Water and drought", "Wildfires",
	"Earthquakes", "Pandemics and vaccines", "Cancer research", "Longevity and aging",
	"Board games", "Japanese culture", "Books and literature", "History of computing",
	"Design systems and UI design", "CSS and frontend development", "JavaScript frameworks",
	"Python packaging", "Go programming language", "Java and the JVM", "Data visualization",
	"Maps and GIS", "Satellite internet", "Drones", "Robotics", "Self-driving cars",
	"AI chatbots and large language models", "Image generation models",
	"Copyright and AI training data", "Elections and politics", "War in Ukraine",
	"China economy", "European Union policy", "India technology sector", "Africa startups",
	"Ocean exploration", "Birds and wildlife", "Dogs and pets", "Coffee", "Wine",
	"Running and fitness", "Cycling races", "Mountaineering", "Sailing",
}

// Reference turns embeddings into the vectors Radar matches with. A topic q
// and a finding with article embedding d and title embedding t score
//
//	(q − c) · (d + t) / 2
//
// where c is the mean embedding of referenceTopics. That is the average of
// the topic's similarity to the article and to the title, minus the same
// average for the reference topics. The title keeps a long article tied to
// what it is about; the subtraction discounts findings that resemble every
// topic. Because the score is a plain inner product of a topic-side and a
// finding-side vector, both are stored ready to multiply and the preview can
// use an index.
//
// c is embedded on first use and kept for the life of the process.
type Reference struct {
	embedder embeddings.Client

	mu       sync.Mutex
	centroid []float32
}

func NewReference(embedder embeddings.Client) *Reference {
	return &Reference{embedder: embedder}
}

// Centroid returns c, embedding the reference topics the first time. A
// failure is not cached, so the next call tries again.
func (r *Reference) Centroid(ctx context.Context) ([]float32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.centroid != nil {
		return r.centroid, nil
	}

	var sum []float64
	for _, topic := range referenceTopics {
		vec, err := r.embedder.Embed(ctx, topic)
		if err != nil {
			return nil, fmt.Errorf("embed reference topic %q: %w", topic, err)
		}

		if sum == nil {
			sum = make([]float64, len(vec))
		}
		if len(vec) != len(sum) {
			return nil, fmt.Errorf("reference topic %q: %d dimensions, want %d", topic, len(vec), len(sum))
		}

		for i, x := range vec {
			sum[i] += float64(x)
		}
	}

	centroid := make([]float32, len(sum))
	for i, x := range sum {
		centroid[i] = float32(x / float64(len(referenceTopics)))
	}

	r.centroid = centroid
	return centroid, nil
}

// TopicMatchVector returns q − c for a topic embedding q.
func (r *Reference) TopicMatchVector(ctx context.Context, topic []float32) ([]float32, error) {
	c, err := r.Centroid(ctx)
	if err != nil {
		return nil, err
	}

	if len(c) != len(topic) {
		return nil, fmt.Errorf("topic embedding has %d dimensions, reference %d", len(topic), len(c))
	}

	out := make([]float32, len(topic))
	for i := range topic {
		out[i] = topic[i] - c[i]
	}

	return out, nil
}

// FindingMatchVector returns (d + t) / 2 for a finding's article embedding d
// and title embedding t. When the finding has no title, or was embedded from
// its title alone, pass its one embedding as both.
func FindingMatchVector(article, title []float32) []float32 {
	out := make([]float32, len(article))
	for i := range article {
		out[i] = (article[i] + title[i]) / 2
	}

	return out
}
