package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func (s *Store) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("postgres: marshal audit event details: %w", err)
	}

	if _, err := s.pool.Exec(ctx, `
		INSERT INTO audit_events (
			id, actor_subject, actor_role, event_type, resource_type, resource_id, request_id, details, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`, event.ID, event.ActorSubject, event.ActorRole, event.EventType, event.ResourceType, event.ResourceID, event.RequestID, details, event.CreatedAt); err != nil {
		return fmt.Errorf("postgres: insert audit event: %w", err)
	}

	return nil
}

func (s *Store) ListAuditEvents(ctx context.Context, resourceType string, resourceID *uuid.UUID, limit int) ([]domain.AuditEvent, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, actor_subject, actor_role, event_type, resource_type, resource_id, request_id, details, created_at
		FROM audit_events
		WHERE ($1 = '' OR resource_type = $1)
		  AND ($2::uuid IS NULL OR resource_id = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := s.pool.Query(ctx, query, resourceType, resourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list audit events: %w", err)
	}
	defer rows.Close()

	var events []domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		var detailsBytes []byte
		if err := rows.Scan(
			&event.ID,
			&event.ActorSubject,
			&event.ActorRole,
			&event.EventType,
			&event.ResourceType,
			&event.ResourceID,
			&event.RequestID,
			&detailsBytes,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan audit event: %w", err)
		}

		if len(detailsBytes) > 0 {
			if err := json.Unmarshal(detailsBytes, &event.Details); err != nil {
				return nil, fmt.Errorf("postgres: unmarshal audit event details: %w", err)
			}
		} else {
			event.Details = map[string]any{}
		}

		events = append(events, event)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("postgres: iterate audit events: %w", rows.Err())
	}

	return events, nil
}
