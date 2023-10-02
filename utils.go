package configuration

import (
	"github.com/bmatcuk/doublestar/v4"
	"log"
	"path"
	"path/filepath"
)

func NormalizeFileName(fileName string) string {
	res, err := filepath.Abs(path.Join("/", fileName))
	if err != nil {
		log.Fatalf("Failed to convert path to absolute: %v", err)
	}
	return res
}

func MatchIncludeExcludePatternsToFile(fileToMatch string, includePatterns []string, excludePatterns []string) bool {
	fileToMatch = NormalizeFileName(fileToMatch)

	for i, _ := range includePatterns {
		includePatterns[i] = NormalizeFileName(includePatterns[i])
	}
	for i, _ := range excludePatterns {
		excludePatterns[i] = NormalizeFileName(excludePatterns[i])
	}

	matching := false
	for _, ipattern := range includePatterns {
		log.Printf("MatchIncludeExcludePatternsToFile: file: %v, include pattern: %v\n", fileToMatch, ipattern)
		isMatched, err := doublestar.PathMatch(ipattern, fileToMatch)
		if err != nil {
			log.Fatalf("Failed to match modified files (%v, %v): Error: %v", fileToMatch, ipattern, err)
		}
		if isMatched {
			matching = true
			break
		}
	}

	for _, epattern := range excludePatterns {
		log.Printf("MatchIncludeExcludePatternsToFile: file: %v, exclude pattern: %v\n", fileToMatch, epattern)
		excluded, err := doublestar.PathMatch(epattern, fileToMatch)
		if err != nil {
			log.Fatalf("Failed to match modified files (%v, %v): Error: %v", fileToMatch, epattern, err)
		}
		if excluded {
			matching = false
			break
		}
	}

	log.Printf("MatchIncludeExcludePatternsToFile: file: %v, match: %v\n", fileToMatch, matching)
	return matching
}
