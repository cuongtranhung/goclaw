package channels

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	minio_storage "github.com/nextlevelbuilder/goclaw/internal/storage/minio"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// RunContext tracks an active agent run for streaming/reaction event forwarding.
type RunContext struct {
	ChannelName  string
	ChatID       string
	MessageID    int
	mu           sync.Mutex
	streamBuffer string // accumulated streaming text (chunks are deltas)
	inToolPhase  bool   // true after tool.call, reset on next chunk (new LLM iteration)
}

// Manager manages all registered channels, handling their lifecycle
// and routing outbound messages to the correct channel.
type Manager struct {
	channels     map[string]Channel
	bus          *bus.MessageBus
	runs         sync.Map // runID string → *RunContext
	dispatchTask *asyncTask
	mu           sync.RWMutex

	// minioMedia, when non-nil, auto-uploads temp media files to MinIO before dispatch.
	minioMedia *minio_storage.MediaStore
}

// SetMinioMediaStore enables automatic upload of temp media files to MinIO.
// When set, media items with local file paths are uploaded and their URLs
// replaced with presigned MinIO URLs before being sent to channels.
func (m *Manager) SetMinioMediaStore(store *minio_storage.MediaStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minioMedia = store
}

type asyncTask struct {
	cancel context.CancelFunc
}

// NewManager creates a new channel manager.
// Channels are registered externally via RegisterChannel.
func NewManager(msgBus *bus.MessageBus) *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		bus:      msgBus,
	}
}

// StartAll starts all registered channels and the outbound dispatch loop.
// The dispatcher is always started even when no channels exist yet,
// because channels may be loaded dynamically later via Reload().
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Always start the outbound dispatcher — channels may be added later via Reload().
	dispatchCtx, cancel := context.WithCancel(ctx)
	m.dispatchTask = &asyncTask{cancel: cancel}
	go m.dispatchOutbound(dispatchCtx)

	if len(m.channels) == 0 {
		slog.Warn("no channels enabled")
		return nil
	}

	slog.Info("starting all channels")

	for name, channel := range m.channels {
		slog.Info("starting channel", "channel", name)
		if err := channel.Start(ctx); err != nil {
			slog.Error("failed to start channel", "channel", name, "error", err)
		}
	}

	slog.Info("all channels started")
	return nil
}

// StopAll gracefully stops all channels and the outbound dispatch loop.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("stopping all channels")

	if m.dispatchTask != nil {
		m.dispatchTask.cancel()
		m.dispatchTask = nil
	}

	for name, channel := range m.channels {
		slog.Info("stopping channel", "channel", name)
		if err := channel.Stop(ctx); err != nil {
			slog.Error("error stopping channel", "channel", name, "error", err)
		}
	}

	slog.Info("all channels stopped")
	return nil
}

// dispatchOutbound consumes outbound messages from the bus and routes them
// to the appropriate channel. Internal channels are silently skipped.
func (m *Manager) dispatchOutbound(ctx context.Context) {
	slog.Info("outbound dispatcher started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbound dispatcher stopped")
			return
		default:
			msg, ok := m.bus.SubscribeOutbound(ctx)
			if !ok {
				continue
			}

			// Skip internal channels
			if IsInternalChannel(msg.Channel) {
				continue
			}

			m.mu.RLock()
			channel, exists := m.channels[msg.Channel]
			m.mu.RUnlock()

			if !exists {
				slog.Warn("unknown channel for outbound message", "channel", msg.Channel)
				continue
			}

			// MinIO backend: upload local temp files to MinIO and replace with presigned URLs.
			// This allows channels (e.g. Telegram) to fetch media via URL instead of local path,
			// and the files persist in MinIO beyond the send operation.
			m.mu.RLock()
			minioMedia := m.minioMedia
			m.mu.RUnlock()
			if minioMedia != nil {
				for i := range msg.Media {
					if !isHTTPURL(msg.Media[i].URL) {
						if presignedURL, err := minioMedia.UploadMedia(ctx, msg.Media[i].URL, msg.Media[i].ContentType); err == nil {
							msg.Media[i].URL = presignedURL
						} else {
							slog.Warn("MinIO media upload failed, using local file",
								"path", msg.Media[i].URL, "error", err)
						}
					}
				}
			}

			slog.Debug("outbound: dispatching to channel", "channel", msg.Channel, "chat_id", msg.ChatID, "content_len", len(msg.Content))
			if err := channel.Send(ctx, msg); err != nil {
				slog.Error("error sending message to channel",
					"channel", msg.Channel,
					"error", err,
				)
			}

			// Clean up temporary media files after successful (or failed) send.
			// Files are created by tools (create_image, tts) and only needed for the send.
			// Skip cleanup when the URL is already an HTTP(S) address (uploaded to MinIO).
			for _, media := range msg.Media {
				if media.URL != "" && !isHTTPURL(media.URL) {
					if err := os.Remove(media.URL); err != nil {
						slog.Debug("failed to clean up media file", "path", media.URL, "error", err)
					}
				}
			}
		}
	}
}

// GetChannel returns a channel by name.
func (m *Manager) GetChannel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	channel, ok := m.channels[name]
	return channel, ok
}

// GetStatus returns the running status of all channels.
func (m *Manager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]interface{})
	for name, channel := range m.channels {
		status[name] = map[string]interface{}{
			"enabled": true,
			"running": channel.IsRunning(),
		}
	}
	return status
}

// GetEnabledChannels returns the names of all enabled channels.
func (m *Manager) GetEnabledChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// RegisterChannel adds a channel to the manager.
func (m *Manager) RegisterChannel(name string, channel Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[name] = channel
}

// UnregisterChannel removes a channel from the manager.
func (m *Manager) UnregisterChannel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, name)
}

// SendToChannel delivers a message to a specific channel by name.
func (m *Manager) SendToChannel(ctx context.Context, channelName, chatID, content string) error {
	m.mu.RLock()
	channel, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel %s not found", channelName)
	}

	msg := bus.OutboundMessage{
		Channel: channelName,
		ChatID:  chatID,
		Content: content,
	}

	return channel.Send(ctx, msg)
}

// --- Run tracking for streaming/reaction event forwarding ---

// RegisterRun associates a run ID with a channel context so agent events
// (chunks, tool calls, completion) can be forwarded to the originating channel.
func (m *Manager) RegisterRun(runID, channelName, chatID string, messageID int) {
	m.runs.Store(runID, &RunContext{
		ChannelName: channelName,
		ChatID:      chatID,
		MessageID:   messageID,
	})
}

// UnregisterRun removes a run tracking entry.
func (m *Manager) UnregisterRun(runID string) {
	m.runs.Delete(runID)
}

// IsStreamingChannel checks if a named channel implements StreamingChannel
// AND has streaming currently enabled in its config (StreamEnabled() == true).
func (m *Manager) IsStreamingChannel(channelName string) bool {
	m.mu.RLock()
	ch, exists := m.channels[channelName]
	m.mu.RUnlock()
	if !exists {
		return false
	}
	sc, ok := ch.(StreamingChannel)
	if !ok {
		return false
	}
	return sc.StreamEnabled()
}

// HandleAgentEvent routes agent lifecycle events to streaming/reaction channels.
// Called from the bus event subscriber — must be non-blocking.
// eventType: "run.started", "chunk", "tool.call", "tool.result", "run.completed", "run.failed"
func (m *Manager) HandleAgentEvent(eventType, runID string, payload interface{}) {
	val, ok := m.runs.Load(runID)
	if !ok {
		return
	}
	rc := val.(*RunContext)

	m.mu.RLock()
	ch, exists := m.channels[rc.ChannelName]
	m.mu.RUnlock()
	if !exists {
		return
	}

	ctx := context.Background()

	// Forward to StreamingChannel
	if sc, ok := ch.(StreamingChannel); ok {
		switch eventType {
		case protocol.AgentEventRunStarted:
			// When the channel supports progress display, skip OnStreamStart here.
			// "thinking" will be shown in the progress list instead, and the
			// DraftStream will be created on demand when the first chunk arrives.
			if _, hasProgress := ch.(ProgressChannel); !hasProgress {
				if err := sc.OnStreamStart(ctx, rc.ChatID); err != nil {
					slog.Debug("stream start failed", "channel", rc.ChannelName, "error", err)
				}
			}
		case protocol.AgentEventToolCall:
			// Agent is executing a tool — mark tool phase so the next chunk
			// (new LLM iteration) resets the stream buffer.
			// Stop the current DraftStream (if any) so the next iteration
			// starts a fresh streaming message.
			rc.mu.Lock()
			rc.inToolPhase = true
			rc.mu.Unlock()
			if err := sc.OnStreamEnd(ctx, rc.ChatID, ""); err != nil {
				slog.Debug("stream tool-phase end failed", "channel", rc.ChannelName, "error", err)
			}
		case protocol.ChatEventChunk:
			// Accumulate chunk deltas into full text.
			// When entering a new LLM iteration (first chunk after tool.call),
			// reset the buffer so we don't concatenate text from previous iterations.
			content := extractPayloadString(payload, "content")
			if content != "" {
				rc.mu.Lock()
				if rc.inToolPhase {
					// New LLM iteration — reset buffer and start fresh stream
					rc.streamBuffer = ""
					rc.inToolPhase = false
					rc.mu.Unlock()
					// Create new DraftStream for this iteration
					if err := sc.OnStreamStart(ctx, rc.ChatID); err != nil {
						slog.Debug("stream restart failed", "channel", rc.ChannelName, "error", err)
					}
					rc.mu.Lock()
				}
				rc.streamBuffer += content
				fullText := rc.streamBuffer
				rc.mu.Unlock()
				if err := sc.OnChunkEvent(ctx, rc.ChatID, fullText); err != nil {
					slog.Debug("stream chunk failed", "channel", rc.ChannelName, "error", err)
				}
			}
		case protocol.AgentEventRunCompleted:
			rc.mu.Lock()
			finalText := rc.streamBuffer
			rc.mu.Unlock()
			if err := sc.OnStreamEnd(ctx, rc.ChatID, finalText); err != nil {
				slog.Debug("stream end failed", "channel", rc.ChannelName, "error", err)
			}
		case protocol.AgentEventRunFailed:
			// Clean up streaming state
			_ = sc.OnStreamEnd(ctx, rc.ChatID, "")
		}
	}

	// Forward to ProgressChannel — maintains the tool-action list in chat.
	if progressCh, ok := ch.(ProgressChannel); ok {
		switch eventType {
		case protocol.AgentEventRunStarted:
			// Show "thinking" as the first item. The placeholder message sent by
			// handlers.go becomes the progress list message.
			if err := progressCh.OnProgressEvent(ctx, rc.ChatID, "_thinking_"); err != nil {
				slog.Debug("progress thinking failed", "channel", rc.ChannelName, "error", err)
			}
		case protocol.AgentEventToolCall:
			// Append the tool to the list after the StreamingChannel has stopped
			// the current DraftStream (giving its message back to placeholders).
			toolName := extractPayloadString(payload, "name")
			if err := progressCh.OnProgressEvent(ctx, rc.ChatID, toolName); err != nil {
				slog.Debug("progress event failed", "channel", rc.ChannelName, "tool", toolName, "error", err)
			}
		case protocol.AgentEventRunCompleted, protocol.AgentEventRunFailed:
			// Clean up any remaining progress list (e.g., run failed before any chunks).
			if err := progressCh.OnProgressClear(ctx, rc.ChatID); err != nil {
				slog.Debug("progress clear failed", "channel", rc.ChannelName, "error", err)
			}
		}
	}

	// Handle LLM retry: update placeholder to notify user
	if eventType == protocol.AgentEventRunRetrying {
		attempt := extractPayloadString(payload, "attempt")
		maxAttempts := extractPayloadString(payload, "maxAttempts")
		errStr := extractPayloadString(payload, "error")

		// Shorten common long error messages for chat display
		displayErr := errStr
		if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429") {
			displayErr = "Rate limit reached"
		} else if strings.Contains(errStr, "503") || strings.Contains(errStr, "overloaded") {
			displayErr = "Provider overloaded"
		} else if len(displayErr) > 60 {
			displayErr = displayErr[:57] + "..."
		}

		retryMsg := fmt.Sprintf("Provider busy (%s), retrying... (%s/%s)", displayErr, attempt, maxAttempts)
		m.bus.PublishOutbound(bus.OutboundMessage{
			Channel: rc.ChannelName,
			ChatID:  rc.ChatID,
			Content: retryMsg,
			Metadata: map[string]string{
				"placeholder_update": "true",
			},
		})
	}

	// Forward to ReactionChannel
	if reactionCh, ok := ch.(ReactionChannel); ok {
		status := ""
		switch eventType {
		case protocol.AgentEventRunStarted:
			status = "thinking"
		case protocol.AgentEventToolCall:
			status = "tool"
		case protocol.AgentEventRunCompleted:
			status = "done"
		case protocol.AgentEventRunFailed:
			status = "error"
		}
		if status != "" {
			if err := reactionCh.OnReactionEvent(ctx, rc.ChatID, rc.MessageID, status); err != nil {
				slog.Debug("reaction event failed", "channel", rc.ChannelName, "status", status, "error", err)
			}
		}
	}

	// Clean up on terminal events
	if eventType == protocol.AgentEventRunCompleted || eventType == protocol.AgentEventRunFailed {
		m.runs.Delete(runID)
	}
}

// isHTTPURL reports whether s is an HTTP or HTTPS URL.
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractPayloadString extracts a string field from a payload (map[string]string or map[string]interface{}).
func extractPayloadString(payload interface{}, key string) string {
	switch p := payload.(type) {
	case map[string]string:
		return p[key]
	case map[string]interface{}:
		if v, ok := p[key].(string); ok {
			return v
		}
	}
	return ""
}
