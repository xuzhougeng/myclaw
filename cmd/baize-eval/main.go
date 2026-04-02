package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"baize/eval"
	"baize/internal/ai"
	"baize/internal/modelconfig"
)

func main() {
	dataDir := flag.String("data-dir", "data", "data directory")
	dataset := flag.String("dataset", "", "dataset file path (e.g., docs/evals/route-command.jsonl)")
	output := flag.String("output", "", "output file path (default: docs/evals/runs/<timestamp>-<dataset>.json)")
	flag.Parse()

	if *dataset == "" {
		log.Fatal("missing -dataset flag")
	}

	ctx := context.Background()
	modelStore := modelconfig.NewStore(
		filepath.Join(*dataDir, "app.db"),
		filepath.Join(*dataDir, "model", "secret.key"),
	)
	aiService := ai.NewService(modelStore)

	cfg, err := modelStore.Load(ctx)
	if err != nil {
		log.Fatalf("load model config: %v", err)
	}

	cases, err := eval.LoadDataset(*dataset)
	if err != nil {
		log.Fatalf("load dataset: %v", err)
	}

	demoTools := eval.DemoTools()
	report := eval.RunReport{
		Dataset:   *dataset,
		Provider:  cfg.Provider,
		Model:     cfg.Model,
		APIType:   cfg.APIType,
		StartedAt: time.Now().Format(time.RFC3339),
		Cases:     make([]eval.CaseResult, 0, len(cases)),
	}

	for i, tc := range cases {
		log.Printf("[%d/%d] running %s", i+1, len(cases), tc.ID)
		result, err := eval.RunStage(ctx, aiService, tc, demoTools)
		if err != nil {
			log.Printf("  error: %v", err)
			result.Error = err.Error()
		} else if result.Pass {
			log.Printf("  ✓ pass")
		} else {
			log.Printf("  ✗ fail: %v", result.Judge.Fields)
		}
		report.Cases = append(report.Cases, result)
	}

	outputPath := *output
	if outputPath == "" {
		timestamp := time.Now().Format("2006-01-02")
		base := filepath.Base(*dataset)
		outputPath = filepath.Join("eval", "testdata", "runs", fmt.Sprintf("%s-%s-%s-%s.json", timestamp, cfg.Provider, cfg.Model, base[:len(base)-6]))
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("marshal report: %v", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		log.Fatalf("write report: %v", err)
	}

	passed := 0
	for _, c := range report.Cases {
		if c.Pass {
			passed++
		}
	}
	log.Printf("done: %d/%d passed, report saved to %s", passed, len(report.Cases), outputPath)
}
