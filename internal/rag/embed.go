package rag

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Embedding progress is reported to the UI so a backfill of a large legacy corpus is
// visible instead of running silently for minutes.
type EmbedState struct {
	Running    bool   `json:"running"`
	Embedded   int64  `json:"embedded"`
	Total      int64  `json:"total"`
	Pending    int64  `json:"pending"`
	WrongSize  int64  `json:"wrong_size"`
	Batches    int    `json:"batches_done"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	LastError  string `json:"last_error,omitempty"`
}

const (
	embedBatchSize = 128
	// A single backfill run is capped so a huge corpus cannot burn the whole daily
	// budget in one pass; the job resumes on the next tick.
	embedBatchesPerRun = 40
)

// EmbedStats reports how much of the corpus already carries a usable vector.
func (x *Indexer) EmbedStats() EmbedState {
	state := EmbedState{Dimensions: x.embedDimensions}
	if x.llm != nil {
		state.Model = x.llm.EmbeddingModel()
	}
	embedded, total, wrong, err := x.db.RagEmbeddingCounts(x.embedDimensions)
	if err != nil {
		state.LastError = err.Error()
		return state
	}
	state.Embedded = embedded
	state.Total = total
	state.WrongSize = wrong
	state.Pending = total - embedded + wrong
	return state
}

// EmbedMissing fills in vectors for stored chunks. It only adds data: no article or
// existing vector is ever removed, so the corpus keeps growing.
func (x *Indexer) EmbedMissing(ctx context.Context) (int, error) {
	if x.llm == nil || !x.llm.Configured() {
		return 0, fmt.Errorf("rag: embedding is not configured")
	}
	embedded := 0
	for batch := 0; batch < embedBatchesPerRun; batch++ {
		chunks, err := x.db.RagChunksMissingEmbedding(embedBatchSize, x.embedDimensions)
		if err != nil {
			return embedded, err
		}
		if len(chunks) == 0 {
			break
		}
		texts := make([]string, len(chunks))
		for i, chunk := range chunks {
			texts[i] = chunk.Content
		}
		vectors, err := x.llm.Embed(ctx, texts, x.embedDimensions)
		if err != nil {
			return embedded, err
		}
		if len(vectors) != len(chunks) {
			return embedded, fmt.Errorf("rag: embedding returned %d vectors for %d chunks", len(vectors), len(chunks))
		}
		for i, chunk := range chunks {
			if err := x.db.SetRagChunkEmbedding(chunk.ID, vectors[i]); err != nil {
				return embedded, err
			}
			embedded++
		}
		if ctx.Err() != nil {
			return embedded, ctx.Err()
		}
	}
	if embedded > 0 {
		x.invalidateCorpus()
		log.Printf("rag: embedded %d chunks", embedded)
	}
	return embedded, nil
}

// StartBackfill runs the embedding backfill once in the background. Concurrent calls
// share the running pass.
func (x *Indexer) StartBackfill() bool {
	x.backfillMu.Lock()
	defer x.backfillMu.Unlock()
	if x.backfillRunning {
		return false
	}
	x.backfillRunning = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, err := x.EmbedMissing(ctx)
		if err != nil {
			log.Printf("rag: embedding backfill stopped: %v", err)
		}
		x.backfillMu.Lock()
		x.backfillRunning = false
		x.backfillMu.Unlock()
	}()
	return true
}

// Backfilling reports whether a pass is in flight.
func (x *Indexer) Backfilling() bool {
	x.backfillMu.Lock()
	defer x.backfillMu.Unlock()
	return x.backfillRunning
}

func (x *Indexer) invalidateCorpus() {
	x.indexMu.Lock()
	x.index = nil
	x.indexCount = 0
	x.indexLatest = 0
	x.indexMu.Unlock()
}
