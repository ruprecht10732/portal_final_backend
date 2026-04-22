package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	memorydb "portal_final_backend/platform/adk/memory/db"
	"portal_final_backend/platform/ai/embeddings"
	"portal_final_backend/platform/logger"
)

type Service struct {
	pool            memorydb.DBTX
	queries         *memorydb.Queries
	embeddingClient *embeddings.Client
	genaiClient     *genai.Client
	modelName       string
	log             *logger.Logger
}

func NewService(pool memorydb.DBTX, embedCfg embeddings.Config, llmAPIKey string, llmModel string, log *logger.Logger) (*Service, error) {
	embedClient := embeddings.NewClient(embedCfg)
	
	genaiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  llmAPIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &Service{
		pool:            pool,
		queries:         memorydb.New(pool),
		embeddingClient: embedClient,
		genaiClient:     genaiClient,
		modelName:       llmModel,
		log:             log,
	}, nil
}

// AddSession implements memory.Service.
func (s *Service) AddSession(ctx context.Context, sess session.Session) error {
	summary, err := s.summarizeSession(ctx, sess)
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	emb, err := s.embeddingClient.Embed(ctx, summary)
	if err != nil {
		return fmt.Errorf("embedding failed: %w", err)
	}

	tenantID := uuid.Nil
	if val, err := sess.State().Get("tenant_id"); err == nil {
		if tstr, ok := val.(string); ok {
			if id, err := uuid.Parse(tstr); err == nil {
				tenantID = id
			}
		}
	}

	_, err = s.queries.InsertAgentMemory(ctx, memorydb.InsertAgentMemoryParams{
		UserID:    sess.UserID(),
		TenantID:  pgtype.UUID{Bytes: tenantID, Valid: true},
		AgentName: sess.AppName(),
		Summary:   summary,
		Embedding: pgvector.NewVector(emb),
		SessionID: sess.ID(),
	})
	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	s.log.Info("stored long-term memory for session", "user_id", sess.UserID(), "session_id", sess.ID())
	return nil
}

// Search implements memory.Service.
func (s *Service) Search(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	emb, err := s.embeddingClient.Embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding search query failed: %w", err)
	}

	// For now, we assume tenantID is nil since SearchRequest lacks it.
	tenantID := uuid.Nil

	rows, err := s.queries.SearchAgentMemory(ctx, memorydb.SearchAgentMemoryParams{
		UserID:    req.UserID,
		TenantID:  pgtype.UUID{Bytes: tenantID, Valid: true},
		AgentName: req.AppName,
		Column4:   pgvector.NewVector(emb),
		Limit:     5,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}

	entries := make([]memory.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, memory.Entry{
			Author:    req.AppName,
			Timestamp: row.CreatedAt.Time,
			Content:   &genai.Content{Role: "user", Parts: []*genai.Part{{Text: row.Summary}}},
		})
	}

	return &memory.SearchResponse{
		Memories: entries,
	}, nil
}

func (s *Service) summarizeSession(ctx context.Context, sess session.Session) (string, error) {
	var builder strings.Builder
	for event := range sess.Events().All() {
		if event.Content != nil {
			builder.WriteString(fmt.Sprintf("%s: ", event.Content.Role))
			for _, part := range event.Content.Parts {
				if part.Text != "" {
					builder.WriteString(part.Text)
				}
				if part.FunctionCall != nil {
					builder.WriteString(fmt.Sprintf("[Call %s]", part.FunctionCall.Name))
				}
				if part.FunctionResponse != nil {
					builder.WriteString(fmt.Sprintf("[Response %s]", part.FunctionResponse.Name))
				}
			}
			builder.WriteString("\n")
		}
	}

	prompt := fmt.Sprintf("Summarize the following conversation focusing on factual state changes, constraints, and long-term user preferences:\n\n%s", builder.String())
	resp, err := s.genaiClient.Models.GenerateContent(ctx, s.modelName,
		[]*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: prompt}}}}, nil)
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		return resp.Candidates[0].Content.Parts[0].Text, nil
	}
	return "No summary generated.", nil
}
