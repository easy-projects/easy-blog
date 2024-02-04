package pkg

import ignore "github.com/sabhiram/go-gitignore"

type GitIgnorer interface {
	AddPatterns(patterns ...string) GitIgnorer
	CleanPatterns(patterns ...string) GitIgnorer
	Match(path string) bool
}

type gitIgnorerImpl struct {
	ignore   *ignore.GitIgnore
	patterns map[string]struct{}
}

func NewBlogIgnorer() GitIgnorer {
	return &gitIgnorerImpl{ignore: ignore.CompileIgnoreLines(), patterns: make(map[string]struct{})}
}

func (bi *gitIgnorerImpl) AddPatterns(patterns ...string) GitIgnorer {
	for _, pattern := range patterns {
		bi.patterns[pattern] = struct{}{}
	}
	patterns = make([]string, 0, len(bi.patterns))
	for pattern := range bi.patterns {
		patterns = append(patterns, pattern)
	}
	bi.ignore = ignore.CompileIgnoreLines(patterns...)
	return bi
}

func (bi *gitIgnorerImpl) CleanPatterns(patterns ...string) GitIgnorer {
	for _, pattern := range patterns {
		delete(bi.patterns, pattern)
	}
	patterns = make([]string, 0, len(bi.patterns))
	for pattern := range bi.patterns {
		patterns = append(patterns, pattern)
	}
	bi.ignore = ignore.CompileIgnoreLines(patterns...)
	return bi
}

func (bi *gitIgnorerImpl) Match(path string) bool {
	return bi.ignore.MatchesPath(path)
}
