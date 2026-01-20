package adapters

import platgit "github.com/megamake/megamake/internal/platform/git"

type PlatformGit struct{}

func NewPlatformGit() PlatformGit {
	return PlatformGit{}
}

func (PlatformGit) ChangedFilesSince(root string, ref string) []string {
	return platgit.ChangedFilesSince(root, ref)
}

func (PlatformGit) ChangedFilesInRange(root string, rng string) []string {
	return platgit.ChangedFilesInRange(root, rng)
}
