package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

type JudgeResult struct {
	Pass   bool              `json:"pass"`
	Fields map[string]string `json:"fields"`
}

func Judge(expect, judge json.RawMessage, actual map[string]any) (JudgeResult, error) {
	var expectMap map[string]any
	if err := json.Unmarshal(expect, &expectMap); err != nil {
		return JudgeResult{}, err
	}
	var judgeMap map[string]any
	if err := json.Unmarshal(judge, &judgeMap); err != nil {
		return JudgeResult{}, err
	}

	result := JudgeResult{Pass: true, Fields: make(map[string]string)}
	for field, rule := range judgeMap {
		expectedVal := expectMap[field]
		actualVal := actual[field]
		pass, msg := judgeField(rule, expectedVal, actualVal)
		result.Fields[field] = msg
		if !pass {
			result.Pass = false
		}
	}
	return result, nil
}

func judgeField(rule, expected, actual any) (bool, string) {
	ruleStr, ok := rule.(string)
	if !ok {
		return false, "invalid rule type"
	}

	switch ruleStr {
	case "exact":
		if fmt.Sprint(expected) == fmt.Sprint(actual) {
			return true, "pass"
		}
		return false, fmt.Sprintf("expected=%v actual=%v", expected, actual)

	case "contains":
		expStr := fmt.Sprint(expected)
		actStr := fmt.Sprint(actual)
		if strings.Contains(actStr, expStr) {
			return true, "pass"
		}
		return false, fmt.Sprintf("'%s' not in '%s'", expStr, actStr)

	case "contains_all":
		expList, ok := expected.([]any)
		if !ok {
			return false, "expected must be array"
		}
		actList, ok := actual.([]any)
		if !ok {
			return false, "actual must be array"
		}
		actStr := strings.Join(toStringSlice(actList), " ")
		for _, item := range expList {
			if !strings.Contains(actStr, fmt.Sprint(item)) {
				return false, fmt.Sprintf("missing '%v'", item)
			}
		}
		return true, "pass"

	case "one_of":
		expList, ok := expected.([]any)
		if !ok {
			if fmt.Sprint(expected) == fmt.Sprint(actual) {
				return true, "pass"
			}
			return false, fmt.Sprintf("expected=%v actual=%v", expected, actual)
		}
		for _, item := range expList {
			if fmt.Sprint(item) == fmt.Sprint(actual) {
				return true, "pass"
			}
		}
		return false, fmt.Sprintf("actual=%v not in %v", actual, expList)

	case "non_empty":
		if actual == nil || actual == "" {
			return false, "empty"
		}
		return true, "pass"

	default:
		return false, fmt.Sprintf("unknown rule: %s", ruleStr)
	}
}

func toStringSlice(items []any) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = fmt.Sprint(item)
	}
	return result
}
