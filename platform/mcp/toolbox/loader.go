package toolbox

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
	"portal_final_backend/platform/mcp"
)

type Config struct {
	Tools []ToolDef `yaml:"tools"`
}

type ToolDef struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	SQL         string         `yaml:"sql"`
	Parameters  map[string]any `yaml:"parameters"`
}

type Loader struct {
	pool   *pgxpool.Pool
	server *mcp.Server
	tools  map[string]ToolDef
}

func NewLoader(pool *pgxpool.Pool, server *mcp.Server) *Loader {
	return &Loader{
		pool:   pool,
		server: server,
		tools:  make(map[string]ToolDef),
	}
}

// LoadAndRegister reads the YAML file, registers tools with the MCP server,
// and returns a ToolHandler that executes the SQL queries.
func (l *Loader) LoadAndRegister(path string) (mcp.ToolHandler, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return l.handler, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}

	for _, t := range cfg.Tools {
		if err := l.server.RegisterTool(t.Name, t.Description, t.Parameters); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", t.Name, err)
		}
		l.tools[t.Name] = t
	}

	return l.handler, nil
}

// handler is the ToolHandler that executes declarative SQL queries.
func (l *Loader) handler(ctx context.Context, name string, args map[string]any) (any, error) {
	toolDef, ok := l.tools[name]
	if !ok {
		// Fallback for non-declarative tools
		return fmt.Sprintf("tool %s executed with args %+v (placeholder)", name, args), nil
	}

	// Use pgx NamedArgs
	namedArgs := pgx.NamedArgs{}
	for k, v := range args {
		namedArgs[k] = v
	}

	rows, err := l.pool.Query(ctx, toolDef.SQL, namedArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to execute SQL for tool %s: %w", name, err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		
		rowMap := make(map[string]any)
		for i, fd := range rows.FieldDescriptions() {
			rowMap[string(fd.Name)] = values[i]
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}
