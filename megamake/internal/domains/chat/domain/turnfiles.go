package domain

import "fmt"

// TurnFileSet contains the canonical filenames for a given 1-based turn number.
type TurnFileSet struct {
	UserTextFile             string // e.g. "user_turn_001.txt"
	AssistantPartialTextFile string // e.g. "assistant_turn_001.partial.txt"
	AssistantTextFile        string // e.g. "assistant_turn_001.txt"
	TurnMetricsFile          string // e.g. "turn_001.json"
}

// TurnFiles returns the stable filenames for a given turn number.
//
// Turn numbering is 1-based.
// If turn <= 0, it returns empty strings for all fields.
func TurnFiles(turn int) TurnFileSet {
	if turn <= 0 {
		return TurnFileSet{}
	}
	n := fmt.Sprintf("%03d", turn)
	return TurnFileSet{
		UserTextFile:             "user_turn_" + n + ".txt",
		AssistantPartialTextFile: "assistant_turn_" + n + ".partial.txt",
		AssistantTextFile:        "assistant_turn_" + n + ".txt",
		TurnMetricsFile:          "turn_" + n + ".json",
	}
}
