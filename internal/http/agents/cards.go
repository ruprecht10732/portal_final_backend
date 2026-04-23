// Package agents provides HTTP handlers for Agent-to-Agent (A2A) protocol support.
package agents

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"portal_final_backend/internal/orchestration"
)

// AgentCard is the A2A-standard manifest describing an agent's capabilities.
type AgentCard struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Skills       []Skill  `json:"skills"`
	Endpoint     string   `json:"endpoint"`
}

// Skill describes a single capability exposed by the agent.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RegisterRoutes mounts agent discovery endpoints.
func RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/capabilities", listAgentCapabilities)
	rg.GET("/cards/:agent", getAgentCard)
}

func listAgentCapabilities(c *gin.Context) {
	cards := []AgentCard{
		buildCard("gatekeeper"),
		buildCard("qualifier"),
		buildCard("calculator"),
		buildCard("matchmaker"),
	}
	c.JSON(http.StatusOK, gin.H{"agents": cards})
}

var defaultCapabilities = []string{
	"text-generation",
	"function-calling",
	"tool-use",
}

func getAgentCard(c *gin.Context) {
	agentName := c.Param("agent")
	ws, err := orchestration.LoadAgentWorkspace(agentName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	card := AgentCard{
		Name:         ws.Name,
		Description:  ws.Description,
		Version:      "1.0.0",
		Capabilities: defaultCapabilities,
		Skills: []Skill{
			{Name: ws.Name, Description: ws.Description},
		},
		Endpoint: "/api/v1/agents/invoke/" + agentName,
	}
	c.JSON(http.StatusOK, card)
}

func buildCard(agentName string) AgentCard {
	ws, err := orchestration.LoadAgentWorkspace(agentName)
	if err != nil {
		return AgentCard{Name: agentName, Description: "unknown"}
	}
	return AgentCard{
		Name:         ws.Name,
		Description:  ws.Description,
		Version:      "1.0.0",
		Capabilities: defaultCapabilities,
		Endpoint:     "/api/v1/agents/invoke/" + agentName,
	}
}
