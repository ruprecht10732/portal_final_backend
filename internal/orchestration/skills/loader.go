// Package skills implements on-demand skill loading (progressive context
// disclosure) for the workspace system. Instead of eagerly loading all L3
// resources into every prompt, agents can call load_skill_resource to fetch
// deep context only when needed.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ResourceLoader loads skill resources on demand from the filesystem.
type ResourceLoader struct {
	root   string
	cache  map[string]string
	mu     sync.RWMutex
}

// NewResourceLoader creates a loader for the given workspace root directory.
func NewResourceLoader(root string) *ResourceLoader {
	return &ResourceLoader{
		root:  root,
		cache: make(map[string]string),
	}
}

// LoadResource fetches a skill resource by path relative to the workspace root.
// Supported resource types: references/*.md, skills/*.md, assets/*.json
func (l *ResourceLoader) LoadResource(resourcePath string) (string, error) {
	// Sanitize path to prevent directory traversal
	resourcePath = filepath.Clean(resourcePath)
	if strings.Contains(resourcePath, "..") {
		return "", fmt.Errorf("skills: invalid resource path %q", resourcePath)
	}

	l.mu.RLock()
	cached, ok := l.cache[resourcePath]
	l.mu.RUnlock()
	if ok {
		return cached, nil
	}

	fullPath := filepath.Join(l.root, resourcePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("skills: load resource %q: %w", resourcePath, err)
	}

	content := string(data)
	l.mu.Lock()
	l.cache[resourcePath] = content
	l.mu.Unlock()
	return content, nil
}

// LoadSkillInstructions loads the L2 instructions for a named skill.
func (l *ResourceLoader) LoadSkillInstructions(skillName string) (string, error) {
	paths := []string{
		filepath.Join("skills", skillName+".md"),
		filepath.Join("references", skillName+".md"),
	}
	for _, p := range paths {
		content, err := l.LoadResource(p)
		if err == nil {
			return content, nil
		}
	}
	return "", fmt.Errorf("skills: instructions not found for skill %q", skillName)
}

// ResourceDescriptor describes an available resource for the LLM to load.
type ResourceDescriptor struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// ListAvailableResources returns all resources under a workspace directory.
func (l *ResourceLoader) ListAvailableResources() ([]ResourceDescriptor, error) {
	var resources []ResourceDescriptor
	for _, dir := range []string{"references", "skills", "assets"} {
		fullDir := filepath.Join(l.root, dir)
		entries, err := os.ReadDir(fullDir)
		if err != nil {
			continue // skip missing dirs
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			resources = append(resources, ResourceDescriptor{
				Name:        strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
				Path:        path,
				Description: fmt.Sprintf("%s resource: %s", dir, e.Name()),
			})
		}
	}
	return resources, nil
}
