package telegram

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	// defaultStreamThrottle is the minimum delay between message edits (matching TS: 1000ms).
	defaultStreamThrottle = 1000 * time.Millisecond

	// streamMaxChars is the max message length for streaming (Telegram limit).
	streamMaxChars = 4096
)

// DraftStream manages a streaming preview message that gets edited as content arrives.
// Ref: TS src/telegram/draft-stream.ts → createTelegramDraftStream()
//
// State machine:
//
//	NOT_STARTED → first Update() → sendMessage (create) → STREAMING
//	STREAMING   → subsequent Update() → editMessageText (throttled) → STREAMING
//	STREAMING   → Stop() → final editMessageText → STOPPED
//	STREAMING   → Clear() → deleteMessage → DELETED
type DraftStream struct {
	bot             *telego.Bot
	chatID          int64
	messageThreadID int           // forum topic thread ID (0 = no thread)
	messageID       int           // 0 = not yet created
	lastText        string        // last sent text (for dedup)
	throttle        time.Duration // min delay between edits
	lastEdit        time.Time
	mu              sync.Mutex
	stopped         bool
	pending         string // pending text to send (buffered during throttle)
}

// newDraftStream creates a new streaming preview manager.
func newDraftStream(bot *telego.Bot, chatID int64, throttleMs int, messageThreadID int) *DraftStream {
	throttle := defaultStreamThrottle
	if throttleMs > 0 {
		throttle = time.Duration(throttleMs) * time.Millisecond
	}
	return &DraftStream{
		bot:             bot,
		chatID:          chatID,
		messageThreadID: messageThreadID,
		throttle:        throttle,
	}
}

// Update sends or edits the streaming message with the latest text.
// Throttled to avoid hitting Telegram rate limits.
func (ds *DraftStream) Update(ctx context.Context, text string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.stopped {
		return
	}

	// Truncate to Telegram max
	if len(text) > streamMaxChars {
		text = text[:streamMaxChars]
	}

	// Dedup: skip if text unchanged
	if text == ds.lastText {
		return
	}

	ds.pending = text

	// Check throttle
	if time.Since(ds.lastEdit) < ds.throttle {
		return
	}

	ds.flush(ctx)
}

// Flush forces sending the pending text immediately.
func (ds *DraftStream) Flush(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.flush(ctx)
}

// flush sends/edits the pending text (must hold mu lock).
func (ds *DraftStream) flush(ctx context.Context) error {
	if ds.pending == "" || ds.pending == ds.lastText {
		return nil
	}

	text := ds.pending
	htmlText := markdownToTelegramHTML(text)

	if ds.messageID == 0 {
		// First message: send new
		// TS ref: buildTelegramThreadParams() — General topic (1) must be omitted.
		params := &telego.SendMessageParams{
			ChatID:    tu.ID(ds.chatID),
			Text:      htmlText,
			ParseMode: telego.ModeHTML,
		}
		if sendThreadID := resolveThreadIDForSend(ds.messageThreadID); sendThreadID > 0 {
			params.MessageThreadID = sendThreadID
		}
		msg, err := ds.bot.SendMessage(ctx, params)
		if err != nil {
			slog.Debug("stream: failed to send initial message", "error", err)
			return err
		}
		ds.messageID = msg.MessageID
		slog.Debug("stream: draft stream message created", "chat_id", ds.chatID, "message_id", ds.messageID)
	} else {
		// Edit existing message
		editMsg := tu.EditMessageText(tu.ID(ds.chatID), ds.messageID, htmlText)
		editMsg.ParseMode = telego.ModeHTML
		if _, err := ds.bot.EditMessageText(ctx, editMsg); err != nil {
			// Ignore "not modified" errors
			if !messageNotModifiedRe.MatchString(err.Error()) {
				slog.Debug("stream: failed to edit message", "error", err)
			}
		}
	}

	ds.lastText = text
	ds.lastEdit = time.Now()
	return nil
}

// toolShortNames maps tool names to concise Vietnamese labels.
var toolShortNames = map[string]string{
	"_thinking_":     "💭 Suy nghĩ",
	"web_search":     "🔍 Tìm kiếm web",
	"read_file":      "📖 Đọc tệp",
	"write_file":     "✏️ Ghi tệp",
	"edit_file":      "✏️ Chỉnh sửa tệp",
	"list_files":     "📂 Liệt kê tệp",
	"run_bash":       "⚙️ Chạy lệnh shell",
	"create_image":   "🎨 Tạo ảnh",
	"http_request":   "🌐 Gửi HTTP request",
	"think":          "💭 Suy nghĩ",
	"read_url":       "🌐 Đọc URL",
	"web_fetch":      "🌐 Tải trang web",
	"memory_search":  "🧠 Tìm kiếm bộ nhớ",
	"memory_write":   "🧠 Ghi nhớ",
	"memory_delete":  "🧠 Xóa bộ nhớ",
	"memory_get":     "🧠 Đọc bộ nhớ",
	"python":         "🐍 Chạy Python",
	"js":             "⚙️ Chạy JavaScript",
	"send_message":   "📨 Gửi tin nhắn",
	"read_memory":    "🧠 Đọc bộ nhớ",
	"search_memory":  "🧠 Tìm kiếm bộ nhớ",
	"list_memory":    "🧠 Xem bộ nhớ",
	"create_task":    "📋 Tạo nhiệm vụ",
	"update_task":    "📋 Cập nhật nhiệm vụ",
	"list_tasks":     "📋 Xem nhiệm vụ",
	"get_task":       "📋 Xem nhiệm vụ",
	"image_generate": "🎨 Tạo ảnh",
	"tts":            "🔊 Tạo giọng nói",
	"stt":            "🎤 Nhận dạng giọng nói",
	"subagent":       "🤖 Giao việc cho trợ lý",
	"spawn":          "🤖 Tạo trợ lý phụ",
	"delegate":       "🤖 Ủy thác cho trợ lý",
}

// toolShortName returns a concise Vietnamese label for a tool name.
func toolShortName(toolName string) string {
	if name, ok := toolShortNames[toolName]; ok {
		return name
	}
	return "⚙️ " + toolName
}

// formatProgressList formats an accumulated list of tool calls as HTML text.
// All tools except the last are shown as completed (✅), the last as in-progress (⏳).
func formatProgressList(tools []string) string {
	var sb strings.Builder
	sb.WriteString("<b>🔄 Đang thực hiện:</b>\n")
	for i, tool := range tools {
		if i < len(tools)-1 {
			sb.WriteString("✅ ")
		} else {
			sb.WriteString("⏳ ")
		}
		sb.WriteString(toolShortName(tool))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// clearProgressList deletes the progress list message and clears tracking state.
// Must be called before starting a new streaming response.
func (c *Channel) clearProgressList(ctx context.Context, chatID string, id int64) {
	if val, ok := c.progressMsgs.LoadAndDelete(chatID); ok {
		c.toolLists.Delete(chatID)
		_ = c.deleteMessage(ctx, id, val.(int))
	}
}

// OnProgressEvent appends a tool call to the per-chat progress list and updates
// a dedicated progress message in chat. The list persists across multiple tool
// calls and is only removed when the final response starts streaming.
func (c *Channel) OnProgressEvent(ctx context.Context, chatID string, toolName string) error {
	if c.config.StreamMode != "partial" {
		return nil
	}

	// Append tool name to the accumulated list (copy-on-write for safety).
	var tools []string
	if val, ok := c.toolLists.Load(chatID); ok {
		prev := val.([]string)
		tools = make([]string, len(prev), len(prev)+1)
		copy(tools, prev)
	}
	tools = append(tools, toolName)
	c.toolLists.Store(chatID, tools)

	text := formatProgressList(tools)

	id, err := parseRawChatID(chatID)
	if err != nil {
		return err
	}

	// Determine which message to edit:
	// 1. An existing progress list message from a prior tool call.
	// 2. The placeholder/DraftStream message (taken from c.placeholders).
	// 3. Fallback: send a brand-new message.
	progressMsgID := 0
	if val, ok := c.progressMsgs.Load(chatID); ok {
		progressMsgID = val.(int)
	} else if val, ok := c.placeholders.LoadAndDelete(chatID); ok {
		progressMsgID = val.(int)
		c.progressMsgs.Store(chatID, progressMsgID)
	}

	if progressMsgID != 0 {
		editMsg := tu.EditMessageText(tu.ID(id), progressMsgID, text)
		editMsg.ParseMode = telego.ModeHTML
		if _, err := c.bot.EditMessageText(ctx, editMsg); err != nil {
			if !messageNotModifiedRe.MatchString(err.Error()) {
				slog.Debug("progress: failed to edit list message", "tool", toolName, "error", err)
			}
		}
	} else {
		// No existing message — send a new one.
		threadID := 0
		if v, ok := c.threadIDs.Load(chatID); ok {
			threadID = v.(int)
		}
		params := &telego.SendMessageParams{
			ChatID:    tu.ID(id),
			Text:      text,
			ParseMode: telego.ModeHTML,
		}
		if sendThreadID := resolveThreadIDForSend(threadID); sendThreadID > 0 {
			params.MessageThreadID = sendThreadID
		}
		msg, err := c.bot.SendMessage(ctx, params)
		if err != nil {
			slog.Debug("progress: failed to send list message", "tool", toolName, "error", err)
		} else {
			c.progressMsgs.Store(chatID, msg.MessageID)
		}
	}

	return nil
}

// OnProgressClear removes the progress list message and resets all tracking state.
// Called on terminal events (run.failed, run.completed) to ensure cleanup.
func (c *Channel) OnProgressClear(ctx context.Context, chatID string) error {
	id, err := parseRawChatID(chatID)
	if err != nil {
		return err
	}
	c.clearProgressList(ctx, chatID, id)
	return nil
}

// Stop finalizes the stream with a final edit.
func (ds *DraftStream) Stop(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.stopped = true
	return ds.flush(ctx)
}

// Clear stops the stream and deletes the message.
func (ds *DraftStream) Clear(ctx context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.stopped = true
	if ds.messageID != 0 {
		_ = ds.bot.DeleteMessage(ctx, &telego.DeleteMessageParams{
			ChatID:    tu.ID(ds.chatID),
			MessageID: ds.messageID,
		})
		ds.messageID = 0
	}
	return nil
}

// MessageID returns the streaming message ID (0 if not yet created).
func (ds *DraftStream) MessageID() int {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.messageID
}

// --- StreamingChannel implementation ---

// OnStreamStart prepares for streaming. Called both at run start and when the
// LLM resumes generating after tool calls.
// When resuming after tools: deletes the accumulated progress list before
// creating a fresh DraftStream for the final response.
// chatID here is the localKey (composite key with :topic:N suffix for forum topics).
func (c *Channel) OnStreamStart(ctx context.Context, chatID string) error {
	if c.config.StreamMode != "partial" {
		return nil
	}

	id, err := parseRawChatID(chatID)
	if err != nil {
		return err
	}

	// Delete progress list message if the LLM is resuming after tool calls.
	c.clearProgressList(ctx, chatID, id)

	// Delete "Thinking..." placeholder if it still exists.
	if pID, ok := c.placeholders.LoadAndDelete(chatID); ok {
		_ = c.deleteMessage(ctx, id, pID.(int))
	}

	// Look up thread ID stored during handleMessage
	threadID := 0
	if v, ok := c.threadIDs.Load(chatID); ok {
		threadID = v.(int)
	}

	// Create draft stream for this chat
	ds := newDraftStream(c.bot, id, 0, threadID)
	c.streams.Store(chatID, ds)

	return nil
}

// OnChunkEvent updates the streaming message with accumulated content.
// When no DraftStream exists yet (e.g., LLM responds directly without calling
// any tools), it clears the progress list and creates the DraftStream on demand.
func (c *Channel) OnChunkEvent(ctx context.Context, chatID string, fullText string) error {
	if c.config.StreamMode != "partial" {
		return nil
	}

	// Auto-create DraftStream if it doesn't exist yet.
	// This happens when the LLM starts responding without calling any tools first
	// (run.started set up the progress list, but OnStreamStart was never called).
	if _, ok := c.streams.Load(chatID); !ok {
		id, err := parseRawChatID(chatID)
		if err != nil {
			return err
		}
		// Clear progress list (e.g., the "thinking" entry added on run.started).
		c.clearProgressList(ctx, chatID, id)
		// Also delete any remaining "Thinking..." placeholder.
		if pID, ok := c.placeholders.LoadAndDelete(chatID); ok {
			_ = c.deleteMessage(ctx, id, pID.(int))
		}
		threadID := 0
		if v, ok := c.threadIDs.Load(chatID); ok {
			threadID = v.(int)
		}
		ds := newDraftStream(c.bot, id, 0, threadID)
		c.streams.Store(chatID, ds)
	}

	val, ok := c.streams.Load(chatID)
	if !ok {
		return nil
	}

	ds := val.(*DraftStream)
	ds.Update(ctx, fullText)
	return nil
}

// OnStreamEnd finalizes the streaming preview.
// Instead of doing a final edit here, we hand the DraftStream's messageID
// back to the placeholders map so that Send() can edit it with the properly
// formatted final response. This avoids duplicate messages.
func (c *Channel) OnStreamEnd(ctx context.Context, chatID string, _ string) error {
	val, ok := c.streams.Load(chatID)
	if !ok {
		return nil
	}

	ds := val.(*DraftStream)

	// Mark stream as stopped (no more edits)
	ds.mu.Lock()
	ds.stopped = true
	msgID := ds.messageID
	ds.mu.Unlock()

	c.streams.Delete(chatID)

	// Hand the DraftStream message back as a placeholder so Send() will
	// edit it with the final formatted content instead of creating a new message.
	slog.Debug("stream: OnStreamEnd", "chat_id", chatID, "draft_msg_id", msgID)
	if msgID != 0 {
		c.placeholders.Store(chatID, msgID)
	}

	// Stop thinking animation
	if stop, ok := c.stopThinking.Load(chatID); ok {
		if cf, ok := stop.(*thinkingCancel); ok {
			cf.Cancel()
		}
		c.stopThinking.Delete(chatID)
	}

	return nil
}
