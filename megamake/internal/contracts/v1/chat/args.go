package chat

// ArgsV1 is the on-disk args.json contract for a chat run.
// This is intended to be stable and easy to inspect.
//
// Notes:
//   - Timestamp should be RFC3339Nano (UTC recommended).
//   - Provider/Model are the selected runtime values for the run.
//   - SystemText/DeveloperText may also appear in MetaV1, but ArgsV1 is the
//     initial snapshot at run creation time.
type ArgsV1 struct {
	Title         string `json:"title"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	SystemText    string `json:"systemText,omitempty"`
	DeveloperText string `json:"developerText,omitempty"`

	// Timestamp is when the run was created (RFC3339Nano).
	Timestamp string `json:"timestamp"`
}
