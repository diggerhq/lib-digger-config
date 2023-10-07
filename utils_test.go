package configuration

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"path"
	"testing"
)

// TODO: remove it
func init() {
	log.SetOutput(os.Stdout)
}

func TestMatchIncludeExcludePatternsToFile(t *testing.T) {
	includePatterns := []string{"projects/dev/**/*"}
	excludePatterns := []string{"projects/dev/project"}
	result := MatchIncludeExcludePatternsToFile("/projects/dev/test1", includePatterns, excludePatterns)
	assert.Equal(t, true, result)

	result = MatchIncludeExcludePatternsToFile("/projects/dev/test/test1", includePatterns, excludePatterns)
	assert.Equal(t, true, result)

	result = MatchIncludeExcludePatternsToFile("/dev/test1", includePatterns, excludePatterns)
	assert.Equal(t, false, result)

	result = MatchIncludeExcludePatternsToFile("projects/dev/project", includePatterns, excludePatterns)
	assert.Equal(t, false, result)
}

func AtlantisFileMatch(fileToMatch string, projectDir string, includePatterns []string, excludePatterns []string) bool {
	log.Printf("fileToMatch: %v\n", fileToMatch)
	log.Printf("includePatterns: %v\n", includePatterns)
	fileToMatch = NormalizeFileName(fileToMatch)
	for i, _ := range includePatterns {
		includePatterns[i] = path.Join(projectDir, NormalizeFileName(includePatterns[i]))
	}
	for i, _ := range excludePatterns {
		excludePatterns[i] = NormalizeFileName(excludePatterns[i])
	}

	log.Printf("includePatterns: %v\n", includePatterns)

	matching := false
	for _, ipattern := range includePatterns {
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
		excluded, err := doublestar.PathMatch(epattern, fileToMatch)
		if err != nil {
			log.Fatalf("Failed to match modified files (%v, %v): Error: %v", fileToMatch, epattern, err)
		}
		if excluded {
			matching = false
			break
		}
	}

	return matching
}

func TestAtlantisFileMatch(t *testing.T) {
	result := AtlantisFileMatch("projects/dev/project/terragrunt.hcl", "projects/dev/project", []string{"*.hcl"}, []string{})
	assert.Equal(t, false, result)

	result = AtlantisFileMatch("projects/terragrunt.hcl", "projects/dev/project", []string{"../../*.hcl"}, []string{})
	assert.Equal(t, false, result)
}
