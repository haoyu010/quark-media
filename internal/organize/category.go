//go:build ignore
// +build ignore

package organize

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// CategoryRule defines a classification rule
type CategoryRule struct {
	GenreIDs            string `yaml:"genre_ids,omitempty"`
	OriginalLanguage    string `yaml:"original_language,omitempty"`
	OriginCountry       string `yaml:"origin_country,omitempty"`
	ProductionCountries string `yaml:"production_countries,omitempty"`
	ReleaseYear         string `yaml:"release_year,omitempty"`
}

// CategoryConfig defines classification strategies for movies and TV
type CategoryConfig struct {
	Movie map[string]CategoryRule `yaml:"movie"`
	TV    map[string]CategoryRule `yaml:"tv"`
}

type namedCategoryRule struct {
	name string
	rule CategoryRule
}

// CategoryClassifier classifies media into categories
type CategoryClassifier struct {
	config     CategoryConfig
	movieRules []namedCategoryRule
	tvRules    []namedCategoryRule
}

// NewCategoryClassifier creates a classifier from config file
func NewCategoryClassifier(configPath string) (*CategoryClassifier, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read category config: %w", err)
	}
	return NewCategoryClassifierFromBytes(data)
}

// NewCategoryClassifierFromBytes creates a classifier from YAML content
func NewCategoryClassifierFromBytes(data []byte) (*CategoryClassifier, error) {
	var config CategoryConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse category config: %w", err)
	}

	return &CategoryClassifier{
		config:     config,
		movieRules: buildOrderedRules(config.Movie),
		tvRules:    buildOrderedRules(config.TV),
	}, nil
}

// Classify determines the category for a media item
func (c *CategoryClassifier) Classify(mediaType string, tmdb *TMDBMatchResult) string {
	var rules []namedCategoryRule

	switch mediaType {
	case "movie":
		rules = c.movieRules
		if len(rules) == 0 {
			rules = buildOrderedRules(c.config.Movie)
		}
	case "tv":
		rules = c.tvRules
		if len(rules) == 0 {
			rules = buildOrderedRules(c.config.TV)
		}
	default:
		return "未分类"
	}

	fallback := ""
	for _, item := range rules {
		// 空规则（无任何条件）作为兜底分类：
		// 仅在所有有条件规则都未命中时返回，分类名可由用户自定义。
		if ruleSpecificity(item.rule) == 0 {
			if fallback == "" {
				fallback = item.name
			}
			continue
		}
		if c.matchRule(item.rule, tmdb) {
			return item.name
		}
	}

	if fallback != "" {
		return fallback
	}

	return "未分类"
}

func buildOrderedRules(rules map[string]CategoryRule) []namedCategoryRule {
	ordered := make([]namedCategoryRule, 0, len(rules))
	for name, rule := range rules {
		ordered = append(ordered, namedCategoryRule{name: name, rule: rule})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ruleSpecificity(ordered[i].rule)
		right := ruleSpecificity(ordered[j].rule)
		if left != right {
			return left > right
		}
		return ordered[i].name < ordered[j].name
	})
	return ordered
}

func ruleSpecificity(rule CategoryRule) int {
	score := 0
	if rule.GenreIDs != "" {
		score++
	}
	if rule.OriginalLanguage != "" {
		score++
	}
	if rule.OriginCountry != "" {
		score++
	}
	if rule.ProductionCountries != "" {
		score++
	}
	if rule.ReleaseYear != "" {
		score++
	}
	return score
}

// matchRule checks if a TMDB result matches a category rule
func (c *CategoryClassifier) matchRule(rule CategoryRule, tmdb *TMDBMatchResult) bool {
	// All conditions must match (AND logic)

	if rule.GenreIDs != "" {
		if !c.matchCommaSeparated(rule.GenreIDs, tmdb.GenreIDs) {
			return false
		}
	}

	if rule.OriginalLanguage != "" {
		if !c.matchValue(rule.OriginalLanguage, tmdb.OriginalLanguage) {
			return false
		}
	}

	if rule.OriginCountry != "" {
		if !c.matchCommaSeparated(rule.OriginCountry, tmdb.OriginCountries) {
			return false
		}
	}

	if rule.ReleaseYear != "" {
		if !c.matchYearRange(rule.ReleaseYear, tmdb.Year) {
			return false
		}
	}

	return true
}

// matchCommaSeparated checks if value matches any in comma-separated list
func (c *CategoryClassifier) matchCommaSeparated(ruleValues string, actualValues []string) bool {
	ruleParts := strings.Split(ruleValues, ",")

	for _, rulePart := range ruleParts {
		rulePart = strings.TrimSpace(rulePart)

		// Handle negation (!value)
		if strings.HasPrefix(rulePart, "!") {
			negValue := strings.TrimPrefix(rulePart, "!")
			for _, actual := range actualValues {
				if strings.EqualFold(actual, negValue) {
					return false
				}
			}
			continue
		}

		// Normal match
		for _, actual := range actualValues {
			if strings.EqualFold(actual, rulePart) {
				return true
			}
		}
	}

	return false
}

// matchValue checks if single value matches comma-separated list
func (c *CategoryClassifier) matchValue(ruleValues string, actualValue string) bool {
	parts := strings.Split(ruleValues, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, actualValue) {
			return true
		}
	}
	return false
}

// matchYearRange checks if year falls in range (e.g., "2020-2025" or "2020")
func (c *CategoryClassifier) matchYearRange(rule string, year int) bool {
	if strings.Contains(rule, "-") {
		parts := strings.SplitN(rule, "-", 2)
		var start, end int
		fmt.Sscanf(parts[0], "%d", &start)
		fmt.Sscanf(parts[1], "%d", &end)
		return year >= start && year <= end
	}

	var exactYear int
	fmt.Sscanf(rule, "%d", &exactYear)
	return year == exactYear
}
