package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run drain_queue.go <check|drain>")
		os.Exit(1)
	}

	command := os.Args[1]

	addr := "37.60.242.49:5434"
	password := "MTAlHI1WIXdSIa5qPcMQtmaOFSLqwLcmcxQqitlsPf1mbYxzbJI5MwREdSbX6s8l"
	
	fmt.Printf("Connecting to Redis at %s (TLS enabled)\n", addr)
	
	// Create Redis client with TLS
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	ctx := context.Background()
	
	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("Failed to connect to Redis: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Connected to Redis")

	// Get Asynq queue info
	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr:      addr,
		Password:  password,
		DB:        0,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	})

	// Check all queues
	queues, err := inspector.Queues()
	if err != nil {
		fmt.Printf("Failed to list queues: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nFound %d queue(s):\n", len(queues))
	
	totalPending := 0
	for _, queueName := range queues {
		info, err := inspector.GetQueueInfo(queueName)
		if err != nil {
			fmt.Printf("  %s: error getting info: %v\n", queueName, err)
			continue
		}
		
		fmt.Printf("\n  Queue: %s\n", queueName)
		fmt.Printf("    Pending:    %d\n", info.Size)
		fmt.Printf("    Active:     %d\n", info.Active)
		fmt.Printf("    Scheduled:  %d\n", info.Scheduled)
		fmt.Printf("    Retry:      %d\n", info.Retry)
		fmt.Printf("    Archived:   %d\n", info.Archived)
		fmt.Printf("    Completed:  %d\n", info.Completed)
		fmt.Printf("    Paused:     %v\n", info.Paused)
		
		totalPending += info.Size + info.Scheduled + info.Retry

		// List pending tasks
		if info.Size > 0 {
			fmt.Printf("\n    Pending tasks (showing up to 10):\n")
			tasks, err := inspector.ListPendingTasks(queueName, asynq.PageSize(10))
			if err != nil {
				fmt.Printf("      Error listing tasks: %v\n", err)
			} else {
				for i, task := range tasks {
					fmt.Printf("      %d. Type: %s, ID: %s, Payload: %s\n", 
						i+1, task.Type, task.ID, string(task.Payload))
				}
			}
		}
		
		// List scheduled tasks
		if info.Scheduled > 0 {
			fmt.Printf("\n    Scheduled tasks (showing up to 5):\n")
			tasks, err := inspector.ListScheduledTasks(queueName, asynq.PageSize(5))
			if err != nil {
				fmt.Printf("      Error listing tasks: %v\n", err)
			} else {
				for i, task := range tasks {
					fmt.Printf("      %d. Type: %s, ID: %s, NextProcessAt: %s\n", 
						i+1, task.Type, task.ID, task.NextProcessAt)
				}
			}
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	fmt.Printf("Total tasks requiring processing: %d\n", totalPending)
	fmt.Printf("%s\n", strings.Repeat("=", 60))

	if command == "drain" && totalPending > 0 {
		fmt.Println("\n⚠️  DRAIN MODE: This will delete all pending, scheduled, and retry tasks!")
		fmt.Println("Waiting 5 seconds before proceeding... (Ctrl+C to cancel)")
		time.Sleep(5 * time.Second)
		
		deleted := 0
		for _, queueName := range queues {
			// Delete pending tasks
			pendingDeleted, err := inspector.DeleteAllPendingTasks(queueName)
			if err != nil {
				fmt.Printf("  Error deleting pending tasks from %s: %v\n", queueName, err)
			} else {
				fmt.Printf("  Deleted %d pending tasks from %s\n", pendingDeleted, queueName)
				deleted += pendingDeleted
			}
			
			// Delete scheduled tasks
			scheduledDeleted, err := inspector.DeleteAllScheduledTasks(queueName)
			if err != nil {
				fmt.Printf("  Error deleting scheduled tasks from %s: %v\n", queueName, err)
			} else {
				fmt.Printf("  Deleted %d scheduled tasks from %s\n", scheduledDeleted, queueName)
				deleted += scheduledDeleted
			}
			
			// Delete retry tasks
			retryDeleted, err := inspector.DeleteAllRetryTasks(queueName)
			if err != nil {
				fmt.Printf("  Error deleting retry tasks from %s: %v\n", queueName, err)
			} else {
				fmt.Printf("  Deleted %d retry tasks from %s\n", retryDeleted, queueName)
				deleted += retryDeleted
			}
			
			// Delete archived tasks (optional)
			archivedDeleted, err := inspector.DeleteAllArchivedTasks(queueName)
			if err != nil {
				fmt.Printf("  Error deleting archived tasks from %s: %v\n", queueName, err)
			} else {
				fmt.Printf("  Deleted %d archived tasks from %s\n", archivedDeleted, queueName)
				deleted += archivedDeleted
			}
		}
		
		fmt.Printf("\n✓ Total tasks deleted: %d\n", deleted)
		fmt.Println("Queue is now empty. Ready for v2.0 deployment.")
	} else if command == "drain" {
		fmt.Println("\n✓ Queue is already empty. Ready for v2.0 deployment.")
	}
}
