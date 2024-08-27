package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	dockerClient "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/m1k1o/neko-rooms/internal/room"
	"github.com/m1k1o/neko-rooms/internal/types"
)

type WorkerManagerCtx struct {
	logger      zerolog.Logger
	client      *dockerClient.Client
	roomManager *room.RoomManagerCtx
	ctx         context.Context
	cancel      context.CancelFunc
}

func New(client *dockerClient.Client, roomManager *room.RoomManagerCtx) *WorkerManagerCtx {
	return &WorkerManagerCtx{
		logger:      log.With().Str("module", "worker").Logger(),
		client:      client,
		roomManager: roomManager,
	}
}

func (w *WorkerManagerCtx) Start() {
	w.ctx, w.cancel = context.WithCancel(context.Background())

	go w.checkDeadlines()
	go w.listenEvents()
}

func (w *WorkerManagerCtx) Shutdown() error {
	w.cancel()
	return nil
}

func (w *WorkerManagerCtx) checkDeadlines() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			rooms, err := w.roomManager.List(w.ctx, nil)
			if err != nil {
				w.logger.Error().Err(err).Msg("Failed to list rooms")
				continue
			}

			for _, room := range rooms {
				deadlineStr, ok := room.ContainerLabels["cotester.vb-orchestrator.deadline"]
				if !ok {
					w.logger.Warn().Str("room", room.ID).Msg("Deadline not found in room labels")
					continue
				}

				deadline, err := time.Parse(time.RFC3339, deadlineStr)
				if err != nil {
					w.logger.Error().Err(err).Str("room", room.ID).Msg("Failed to parse deadline")
					continue
				}

				if time.Now().After(deadline) {
					w.logger.Info().Str("room", room.ID).Msg("Room deadline reached, stopping")
					err := w.roomManager.Remove(w.ctx, room.ID)
					if err != nil {
						w.logger.Error().Err(err).Str("room", room.ID).Msg("Failed to stop room")
					}
				}
			}
		}
	}
}

func (w *WorkerManagerCtx) listenEvents() {
	events, errs := w.roomManager.Events(w.ctx)

	for {
		select {
		case <-w.ctx.Done():
			return
		case err := <-errs:
			w.logger.Error().Err(err).Msg("Error from room events")
		case event := <-events:
			if event.Action == types.RoomEventStopped || event.Action == types.RoomEventDestroyed {
				w.updateSessionStatus(&event, "STOPPED")
			} else if event.Action == types.RoomEventStarted {
				w.updateSessionStatus(&event, "RUNNING")
			}
		}
	}
}

func (w *WorkerManagerCtx) updateSessionStatus(event *types.RoomEvent, status string) {
	apiEndpoint, ok := event.ContainerLabels["cotester.vb-orchestrator.api-endpoint"]
	if !ok {
		w.logger.Error().Str("room", event.ID).Msg("API endpoint not found in room labels")
		return
	}

	sessionId, ok := event.ContainerLabels["cotester.vb-orchestrator.session-id"]
	if !ok {
		w.logger.Error().Str("room", event.ID).Msg("Session ID not found in room labels")
		return
	}

	apiKey, ok := event.ContainerLabels["cotester.vb-orchestrator.api-key"]
	if !ok {
		w.logger.Error().Str("room", event.ID).Msg("API key not found in room labels")
		return
	}

	url := fmt.Sprintf("%s/api/v1/sessions", apiEndpoint)
	data := map[string]string{
		"sessionId": sessionId,
		"status":    status,
		"secretKey": apiKey,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		w.logger.Error().Err(err).Str("room", event.ID).Msg("Failed to marshal session update data")
		return
	}

	req, err := http.NewRequestWithContext(w.ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		w.logger.Error().Err(err).Str("room", event.ID).Msg("Failed to create session update request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.logger.Error().Err(err).Str("room", event.ID).Msg("Failed to send session update request")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.logger.Error().Int("status", resp.StatusCode).Str("room", event.ID).Msg("Session update request failed")
	} else {
		w.logger.Info().Str("room", event.ID).Str("status", status).Msg("Session status updated successfully")
	}
}
