package ports

type Git interface {
	ChangedFilesSince(root string, ref string) []string
	ChangedFilesInRange(root string, rng string) []string
}
