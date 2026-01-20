package project

// ProjectProfileV1 is the v1 contract for project detection output.
// It intentionally mirrors the shape of your Swift ProjectProfile in spirit,
// but uses JSON-friendly primitives.
type ProjectProfileV1 struct {
	RootPath     string   `json:"rootPath"`
	Languages    []string `json:"languages"`
	Markers      []string `json:"markers,omitempty"` // relative paths that proved existence
	IsCodeProject bool    `json:"isCodeProject"`
	Why          []string `json:"why,omitempty"`     // human-friendly evidence lines
}
