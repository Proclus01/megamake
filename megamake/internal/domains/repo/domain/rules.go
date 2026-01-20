package domain

// IncludeRules defines what the repo scanner includes/excludes.
type IncludeRules struct {
	AllowedExts        map[string]bool
	ForceIncludeNames  map[string]bool
	ForceIncludeGlobs  []string
	PruneDirs          map[string]bool
	ExcludeNames       map[string]bool
	ExcludeExts        map[string]bool
}

// BuildRules returns language-aware include rules.
// It is intentionally close to your Swift RulesFactory, with conservative defaults.
func BuildRules(languages map[string]bool) IncludeRules {
	allowed := map[string]bool{
		// source
		".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
		".py": true, ".go": true, ".rs": true, ".java": true, ".kt": true, ".kts": true,
		".c": true, ".cc": true, ".cpp": true, ".cxx": true, ".h": true, ".hpp": true, ".hh": true, ".cs": true,
		".php": true, ".rb": true, ".swift": true,
		".lean": true,

		// configs/docs
		".yml": true, ".yaml": true, ".json": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
		".md": true, ".xml": true, ".sql": true, ".graphql": true, ".gql": true,
		".sh": true, ".bash": true, ".zsh": true,
		".html": true, ".css": true, ".scss": true, ".sass": true, ".less": true,

		// latex
		".tex": true, ".cls": true, ".sty": true, ".bib": true,
	}

	excludeNames := map[string]bool{
		"package-lock.json": true,
		"pnpm-lock.yaml":    true,
		"yarn.lock":         true,
		"go.sum":            true,
		"cargo.lock":        true,
		"package.resolved":  true,
		".ds_store":         true,
		".gitignore":        true,
	}

	excludeExts := map[string]bool{
		".pem":   true,
		".crt":   true,
		".key":   true,
		".png":   true,
		".jpg":   true,
		".jpeg":  true,
		".gif":   true,
		".webp":  true,
		".svg":   true,
		".ico":   true,
		".pdf":   true,
		".zip":   true,
		".tar":   true,
		".gz":    true,
		".tgz":   true,
		".xz":    true,
		".7z":    true,
		".rar":   true,
		".so":    true,
		".dylib": true,
		".dll":   true,
		".class": true,
		".jar":   true,
		".war":   true,
		".wasm":  true,
		".map":   true,
	}

	forceNames := map[string]bool{
		"package.json": true,
		"tsconfig.json": true,
		"jsconfig.json": true,
		"go.mod": true,
		"cargo.toml": true,
		"pom.xml": true,
		"build.gradle": true,
		"build.gradle.kts": true,
		"settings.gradle": true,
		"settings.gradle.kts": true,
		"pyproject.toml": true,
		"requirements.txt": true,
		"pipfile": true,
		"setup.py": true,
		"setup.cfg": true,
		"tox.ini": true,
		"dockerfile": true,
		"docker-compose.yml": true,
		"docker-compose.yaml": true,
		"makefile": true,
		"cmakelists.txt": true,
		"lakefile.lean": true,
		"lean-toolchain": true,
		"latexmkrc": true,
		".gitattributes": true,
	}

	forceGlobs := []string{
		".github/workflows/*.yml",
		".github/workflows/*.yaml",
		".github/actions/**/*.yml",
		".github/actions/**/*.yaml",
		".circleci/config.yml",
		".circleci/config.yaml",
		".gitlab-ci.yml",
		"azure-pipelines.yml",
		".github/dependabot.yml",
	}

	prune := map[string]bool{
		"vendor": true, ".expo": true, "node_modules": true, "app-example": true,
		".git": true, ".hg": true, ".svn": true,
		".next": true, ".nuxt": true, ".svelte-kit": true,
		"env": true, "venv": true, ".env": true, ".venv": true,
		"__pycache__": true, ".mypy_cache": true, ".pytest_cache": true, ".ruff_cache": true, ".tox": true,
		".build": true, ".swiftpm": true, "build": true, "dist": true, "out": true, "target": true, "bin": true, "obj": true,
		".idea": true, ".vscode": true, ".gradle": true, ".cache": true, ".parcel-cache": true, ".turbo": true,
		".nyc_output": true, ".coverage": true, "coverage": true,
		"DerivedData": true, "Pods": true,
		".lake": true, "lake-packages": true,
		".terraform": true, "terraform.d": true,
		".docusaurus": true, ".vitepress": true, ".astro": true,
		".yarn": true, ".pnpm-store": true,
		".history": true,
		".direnv": true,
	}

	// Prefer TypeScript over JS/JSX when TypeScript present.
	if languages["typescript"] {
		delete(allowed, ".js")
		delete(allowed, ".jsx")
	}

	return IncludeRules{
		AllowedExts:       allowed,
		ForceIncludeNames: forceNames,
		ForceIncludeGlobs: forceGlobs,
		PruneDirs:         prune,
		ExcludeNames:      excludeNames,
		ExcludeExts:       excludeExts,
	}
}
