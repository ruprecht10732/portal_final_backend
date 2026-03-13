package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	apptools "portal_final_backend/internal/tools"

	"gopkg.in/yaml.v3"
)

const agentWorkspaceRootEnv = "AGENT_WORKSPACE_ROOT"
const loadAgentWorkspaceErrorFormat = "load agent workspace %q: %w"
const readFileErrorFormat = "read %s: %w"
const sharedWorkspaceDir = "agents/shared"
const skillFileName = "SKILL.md"
const agentsFileName = "AGENTS.md"
const referencesDirName = "references"
const promptsDirName = "prompts"
const skillsDirName = "skills"
const contextFileName = "context.md"

type workspaceDefinition struct {
	workspaceDir string
}

type Workspace struct {
	Instruction  string
	AllowedTools []string
	Name         string
	Description  string
}

type parsedSkillFile struct {
	metadata     skillFrontmatter
	body         string
	relativePath string
}

type skillFrontmatter struct {
	Name         string              `yaml:"name"`
	Description  string              `yaml:"description"`
	AllowedTools []string            `yaml:"allowed-tools"`
	Metadata     skillMetadataFields `yaml:"metadata"`
}

type skillMetadataFields struct {
	AllowedTools []string `yaml:"allowed-tools"`
}

var workspaceDefinitions = map[string]workspaceDefinition{
	"gatekeeper":     {workspaceDir: "agents/gatekeeper"},
	"qualifier":      {workspaceDir: "agents/qualifier"},
	"calculator":     {workspaceDir: "agents/calculator"},
	"matchmaker":     {workspaceDir: "agents/matchmaker"},
	"auditor":        {workspaceDir: "agents/support/auditor"},
	"photo-analyzer": {workspaceDir: "agents/support/photo_analyzer"},
	"call-logger":    {workspaceDir: "agents/support/call_logger"},
	"offer-summary":  {workspaceDir: "agents/support/offer_summary"},
	"whatsapp-reply": {workspaceDir: "agents/support/whatsapp_reply"},
	"whatsapp-agent": {workspaceDir: "agents/support/whatsapp_agent"},
	"email-reply":    {workspaceDir: "agents/support/email_reply"},
}

var loggedWorkspaceLoads sync.Map

func LoadAgentWorkspace(agentName string) (Workspace, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(agentName))
	if normalizedName == "" {
		return Workspace{}, errors.New("agent name is required")
	}

	definition, ok := workspaceDefinitions[normalizedName]
	if !ok {
		return Workspace{}, fmt.Errorf("unsupported agent workspace %q", agentName)
	}

	rootDir, err := resolveAgentWorkspaceRoot()
	if err != nil {
		return Workspace{}, err
	}

	return loadAgentWorkspaceFromRoot(rootDir, normalizedName, definition)
}

func ValidateAgentWorkspaces(agentNames ...string) error {
	names := agentNames
	if len(names) == 0 {
		names = make([]string, 0, len(workspaceDefinitions))
		for name := range workspaceDefinitions {
			names = append(names, name)
		}
		sort.Strings(names)
	}

	for _, agentName := range names {
		if _, err := LoadAgentWorkspace(agentName); err != nil {
			return err
		}
	}

	return nil
}

func BuildAgentInstruction(agentName string, extraSections ...string) (string, error) {
	workspace, err := LoadAgentWorkspace(agentName)
	if err != nil {
		return "", err
	}

	sections := []string{workspace.Instruction}
	for _, extraSection := range extraSections {
		trimmed := strings.TrimSpace(extraSection)
		if trimmed == "" {
			continue
		}
		sections = append(sections, trimmed)
	}

	return strings.Join(sections, "\n\n"), nil
}

func MustLoadWorkspaceMarkdownText(relativePath string) string {
	rootDir, err := resolveAgentWorkspaceRoot()
	if err != nil {
		panic(fmt.Sprintf("resolve workspace root for %s: %v", relativePath, err))
	}
	content, err := readAgentWorkspaceFile(rootDir, relativePath)
	if err != nil {
		panic(fmt.Sprintf("read workspace markdown %s: %v", relativePath, err))
	}
	return content
}

func loadAgentWorkspaceFromRoot(rootDir, normalizedName string, definition workspaceDefinition) (Workspace, error) {
	rootInstructionPath, err := resolveRootInstructionsFile(rootDir)
	if err != nil {
		return Workspace{}, err
	}

	sharedSkill, err := loadSkillFile(rootDir, filepath.ToSlash(filepath.Join(sharedWorkspaceDir, skillFileName)))
	if err != nil {
		return Workspace{}, fmt.Errorf(loadAgentWorkspaceErrorFormat, normalizedName, err)
	}

	agentSkillPath := filepath.ToSlash(filepath.Join(definition.workspaceDir, skillFileName))
	agentSkill, err := loadSkillFile(rootDir, agentSkillPath)
	if err != nil {
		return Workspace{}, fmt.Errorf(loadAgentWorkspaceErrorFormat, normalizedName, err)
	}

	sharedResourceFiles, err := resolveSharedResourceFiles(rootDir)
	if err != nil {
		return Workspace{}, fmt.Errorf(loadAgentWorkspaceErrorFormat, normalizedName, err)
	}

	agentResourceFiles, err := resolveAgentResourceFiles(rootDir, definition.workspaceDir)
	if err != nil {
		return Workspace{}, fmt.Errorf(loadAgentWorkspaceErrorFormat, normalizedName, err)
	}
	instruction, loadedFiles, err := composeWorkspaceInstruction(rootDir, normalizedName, rootInstructionPath, sharedSkill, agentSkill, sharedResourceFiles, agentResourceFiles)
	if err != nil {
		return Workspace{}, fmt.Errorf(loadAgentWorkspaceErrorFormat, normalizedName, err)
	}
	logAgentWorkspaceLoadedOnce(normalizedName, rootDir, loadedFiles, instruction)

	return Workspace{
		Instruction:  instruction,
		AllowedTools: skillAllowedTools(agentSkill.metadata),
		Name:         strings.TrimSpace(agentSkill.metadata.Name),
		Description:  strings.TrimSpace(agentSkill.metadata.Description),
	}, nil
}

func composeWorkspaceInstruction(rootDir, normalizedName, rootInstructionPath string, sharedSkill, agentSkill parsedSkillFile, sharedResourceFiles, agentResourceFiles []string) (string, []string, error) {
	sections := make([]string, 0, 4+len(sharedResourceFiles)+len(agentResourceFiles))
	loadedFiles := make([]string, 0, 4+len(sharedResourceFiles)+len(agentResourceFiles))
	seenFiles := make(map[string]struct{})

	appendFileSection := func(relativePath string) error {
		relativePath = filepath.ToSlash(relativePath)
		if _, seen := seenFiles[relativePath]; seen {
			return nil
		}
		content, readErr := readAgentWorkspaceFile(rootDir, relativePath)
		if readErr != nil {
			return readErr
		}
		sections = append(sections, content)
		loadedFiles = append(loadedFiles, relativePath)
		seenFiles[relativePath] = struct{}{}
		return nil
	}

	appendBodySection := func(relativePath, body string) {
		relativePath = filepath.ToSlash(relativePath)
		if _, seen := seenFiles[relativePath]; seen {
			return
		}
		sections = append(sections, body)
		loadedFiles = append(loadedFiles, relativePath)
		seenFiles[relativePath] = struct{}{}
	}

	if err := appendFileSection(rootInstructionPath); err != nil {
		return "", nil, err
	}
	appendBodySection(sharedSkill.relativePath, sharedSkill.body)
	for _, relativePath := range sharedResourceFiles {
		if err := appendFileSection(relativePath); err != nil {
			return "", nil, err
		}
	}
	appendBodySection(agentSkill.relativePath, agentSkill.body)
	for _, relativePath := range agentResourceFiles {
		if err := appendFileSection(relativePath); err != nil {
			return "", nil, err
		}
	}

	sections = append(sections, buildWorkspacePersistenceSection(normalizedName, rootDir, agentSkill.relativePath, loadedFiles))
	instruction := strings.Join(sections, "\n\n")
	return instruction, loadedFiles, nil
}

func resolveSharedResourceFiles(rootDir string) ([]string, error) {
	rootFiles, err := collectWorkspaceRootMarkdownFiles(rootDir, sharedWorkspaceDir, skillFileName)
	if err != nil {
		return nil, err
	}
	promptFiles, err := collectOptionalMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(sharedWorkspaceDir, promptsDirName)))
	if err != nil {
		return nil, err
	}
	skillFiles, err := collectOptionalMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(sharedWorkspaceDir, skillsDirName)))
	if err != nil {
		return nil, err
	}
	referenceFiles, err := collectOptionalMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(sharedWorkspaceDir, referencesDirName)))
	if err != nil {
		return nil, err
	}
	combined := append([]string{}, rootFiles...)
	combined = append(combined, promptFiles...)
	combined = append(combined, skillFiles...)
	combined = append(combined, referenceFiles...)
	return uniqueRelativePaths(combined), nil
}

func resolveAgentResourceFiles(rootDir, workspaceDir string) ([]string, error) {
	contextPath := filepath.ToSlash(filepath.Join(workspaceDir, contextFileName))
	if !fileExists(filepath.Join(rootDir, filepath.FromSlash(contextPath))) {
		return nil, fmt.Errorf("read %s: file not found", contextPath)
	}
	rootMarkdownFiles, err := collectWorkspaceRootMarkdownFiles(rootDir, workspaceDir, skillFileName, contextFileName)
	if err != nil {
		return nil, err
	}
	promptFiles, err := collectOptionalMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(workspaceDir, promptsDirName)))
	if err != nil {
		return nil, err
	}
	skillFiles, err := collectMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(workspaceDir, skillsDirName)))
	if err != nil {
		return nil, err
	}
	if len(skillFiles) == 0 {
		return nil, fmt.Errorf("no skill markdown files found in %s", filepath.ToSlash(filepath.Join(workspaceDir, skillsDirName)))
	}
	referenceFiles, err := collectOptionalMarkdownFiles(rootDir, filepath.ToSlash(filepath.Join(workspaceDir, referencesDirName)))
	if err != nil {
		return nil, err
	}
	combined := []string{contextPath}
	combined = append(combined, rootMarkdownFiles...)
	combined = append(combined, promptFiles...)
	combined = append(combined, skillFiles...)
	combined = append(combined, referenceFiles...)
	return uniqueRelativePaths(combined), nil
}

func resolveAgentWorkspaceRoot() (string, error) {
	if rootOverride := strings.TrimSpace(os.Getenv(agentWorkspaceRootEnv)); rootOverride != "" {
		return validateAgentWorkspaceRoot(rootOverride)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	if rootDir, ok := findWorkspaceRootFrom(cwd); ok {
		return rootDir, nil
	}
	if executablePath, err := os.Executable(); err == nil {
		if rootDir, ok := findWorkspaceRootFrom(filepath.Dir(executablePath)); ok {
			return rootDir, nil
		}
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		if rootDir, ok := findWorkspaceRootFrom(filepath.Dir(sourceFile)); ok {
			return rootDir, nil
		}
	}
	return "", fmt.Errorf("could not locate %s from %s; set %s to the backend root", agentsFileName, filepath.ToSlash(cwd), agentWorkspaceRootEnv)
}

func findWorkspaceRootFrom(startDir string) (string, bool) {
	for dir := startDir; ; dir = filepath.Dir(dir) {
		if rootDir, err := validateAgentWorkspaceRoot(dir); err == nil {
			return rootDir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func validateAgentWorkspaceRoot(rootDir string) (string, error) {
	absoluteRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root %q: %w", rootDir, err)
	}
	if _, err := resolveRootInstructionsFile(absoluteRoot); err != nil {
		return "", err
	}
	return absoluteRoot, nil
}

func resolveRootInstructionsFile(rootDir string) (string, error) {
	absolutePath := filepath.Join(rootDir, agentsFileName)
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("workspace root %q missing %s: %w", filepath.ToSlash(rootDir), agentsFileName, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("workspace root %q has %s directory, expected file", filepath.ToSlash(rootDir), agentsFileName)
	}
	return agentsFileName, nil
}

func loadSkillFile(rootDir, relativePath string) (parsedSkillFile, error) {
	content, err := readAgentWorkspaceFile(rootDir, relativePath)
	if err != nil {
		return parsedSkillFile{}, err
	}
	metadata, body, err := parseSkillFrontmatter(content, relativePath)
	if err != nil {
		return parsedSkillFile{}, err
	}
	return parsedSkillFile{metadata: metadata, body: body, relativePath: filepath.ToSlash(relativePath)}, nil
}

func parseSkillFrontmatter(content, relativePath string) (skillFrontmatter, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return skillFrontmatter{}, "", fmt.Errorf("read %s: missing YAML frontmatter", filepath.ToSlash(relativePath))
	}
	closingIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingIndex = i
			break
		}
	}
	if closingIndex == -1 {
		return skillFrontmatter{}, "", fmt.Errorf("read %s: unterminated YAML frontmatter", filepath.ToSlash(relativePath))
	}
	var metadata skillFrontmatter
	frontmatter := strings.Join(lines[1:closingIndex], "\n")
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return skillFrontmatter{}, "", fmt.Errorf("parse %s frontmatter: %w", filepath.ToSlash(relativePath), err)
	}
	if strings.TrimSpace(metadata.Name) == "" {
		return skillFrontmatter{}, "", fmt.Errorf("read %s: name is required", filepath.ToSlash(relativePath))
	}
	if strings.TrimSpace(metadata.Description) == "" {
		return skillFrontmatter{}, "", fmt.Errorf("read %s: description is required", filepath.ToSlash(relativePath))
	}
	body := strings.TrimSpace(strings.Join(lines[closingIndex+1:], "\n"))
	if body == "" {
		return skillFrontmatter{}, "", fmt.Errorf("read %s: markdown body is required", filepath.ToSlash(relativePath))
	}
	for _, toolName := range skillAllowedTools(metadata) {
		if !apptools.IsKnownTool(toolName) {
			return skillFrontmatter{}, "", fmt.Errorf("read %s: unknown allowed-tool %q", filepath.ToSlash(relativePath), toolName)
		}
	}
	return metadata, body, nil
}

func skillAllowedTools(metadata skillFrontmatter) []string {
	if normalized := normalizeAllowedTools(metadata.Metadata.AllowedTools); len(normalized) > 0 {
		return normalized
	}
	return normalizeAllowedTools(metadata.AllowedTools)
}

func normalizeAllowedTools(allowedTools []string) []string {
	if len(allowedTools) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(allowedTools))
	normalized := make([]string, 0, len(allowedTools))
	for _, toolName := range allowedTools {
		trimmed := strings.TrimSpace(toolName)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func collectWorkspaceRootMarkdownFiles(rootDir, relativeDir string, excludeNames ...string) ([]string, error) {
	absoluteDir := filepath.Join(rootDir, filepath.FromSlash(relativeDir))
	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return nil, fmt.Errorf(readFileErrorFormat, filepath.ToSlash(relativeDir), err)
	}
	excluded := make(map[string]struct{}, len(excludeNames))
	for _, name := range excludeNames {
		excluded[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		if _, skip := excluded[strings.ToLower(entry.Name())]; skip {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join(relativeDir, entry.Name())))
	}
	sort.Strings(files)
	return files, nil
}

func collectOptionalMarkdownFiles(rootDir, relativeDir string) ([]string, error) {
	absoluteDir := filepath.Join(rootDir, filepath.FromSlash(relativeDir))
	if !fileExists(absoluteDir) {
		return nil, nil
	}
	return collectMarkdownFiles(rootDir, relativeDir)
}

func collectMarkdownFiles(rootDir, relativeDir string) ([]string, error) {
	absoluteDir := filepath.Join(rootDir, filepath.FromSlash(relativeDir))
	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return nil, fmt.Errorf(readFileErrorFormat, filepath.ToSlash(relativeDir), err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join(relativeDir, entry.Name())))
	}
	sort.Strings(files)
	return files, nil
}

func uniqueRelativePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		unique = append(unique, normalized)
	}
	return unique
}

func logAgentWorkspaceLoadedOnce(agentName, rootDir string, loadedFiles []string, instruction string) {
	hash := shortInstructionHash(instruction)
	key := agentName + ":" + rootDir + ":" + hash
	if _, loaded := loggedWorkspaceLoads.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	log.Printf("agent workspace loaded agent=%s root=%s hash=%s files=%s", agentName, filepath.ToSlash(rootDir), hash, strings.Join(loadedFiles, ","))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortInstructionHash(instruction string) string {
	sum := sha256.Sum256([]byte(instruction))
	return hex.EncodeToString(sum[:8])
}

func readAgentWorkspaceFile(rootDir, relativePath string) (string, error) {
	fullPath := filepath.Join(rootDir, filepath.FromSlash(relativePath))
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf(readFileErrorFormat, filepath.ToSlash(relativePath), err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return "", fmt.Errorf("read %s: file is empty", filepath.ToSlash(relativePath))
	}
	return trimmed, nil
}

func buildWorkspacePersistenceSection(agentName, rootDir, entryFile string, loadedFiles []string) string {
	var builder strings.Builder
	builder.WriteString("# Workspace Persistence\n\n")
	builder.WriteString("- Active workspace agent: ")
	builder.WriteString(agentName)
	builder.WriteString("\n- Workspace root: ")
	builder.WriteString(filepath.ToSlash(rootDir))
	builder.WriteString("\n- Active entry file: ")
	builder.WriteString(filepath.ToSlash(entryFile))
	builder.WriteString("\n- Loaded markdown files:\n")
	for _, file := range loadedFiles {
		builder.WriteString("  - ")
		builder.WriteString(filepath.ToSlash(file))
		builder.WriteString("\n")
	}
	builder.WriteString("- Source of truth: markdown defines role behavior and tool exposure; Go code enforces runtime invariants and write paths.")
	return builder.String()
}
