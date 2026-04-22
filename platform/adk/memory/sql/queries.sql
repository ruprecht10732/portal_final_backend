-- name: InsertAgentMemory :one
INSERT INTO agent_memory (
    user_id,
    tenant_id,
    agent_name,
    summary,
    embedding,
    session_id
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: SearchAgentMemory :many
SELECT
    id,
    user_id,
    tenant_id,
    agent_name,
    summary,
    session_id,
    created_at,
    1 - (embedding <=> $4::vector) AS similarity
FROM agent_memory
WHERE
    user_id = $1
    AND tenant_id = $2
    AND agent_name = $3
ORDER BY embedding <=> $4::vector
LIMIT $5;

-- name: InsertAgentMemoryMetadata :exec
INSERT INTO agent_memory_metadata (
    memory_id,
    key,
    value
) VALUES (
    $1, $2, $3
);
