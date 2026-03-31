package eval

import (
	"bufio"
	"encoding/json"
	"os"
)

type TestCase struct {
	ID     string          `json:"id"`
	Stage  string          `json:"stage"`
	Input  string          `json:"input,omitempty"`
	Task   string          `json:"task,omitempty"`
	Tools  []string        `json:"tools,omitempty"`
	Prior  json.RawMessage `json:"prior,omitempty"`
	Expect json.RawMessage `json:"expect"`
	Judge  json.RawMessage `json:"judge"`
	Note   string          `json:"note,omitempty"`

	// search_plan specific
	Question string `json:"question,omitempty"`

	// tool_plan specific
	ToolName string `json:"tool_name,omitempty"`

	// agent_step specific
	History json.RawMessage `json:"history,omitempty"`
	Results json.RawMessage `json:"results,omitempty"`

	// agent_loop specific
	State    json.RawMessage `json:"state,omitempty"`
	MaxSteps int             `json:"max_steps,omitempty"`
}

func LoadDataset(path string) ([]TestCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cases []TestCase
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var tc TestCase
		if err := json.Unmarshal(line, &tc); err != nil {
			return nil, err
		}
		cases = append(cases, tc)
	}
	return cases, scanner.Err()
}
