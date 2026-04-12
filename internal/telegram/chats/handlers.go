package chats

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	"mangoduck/internal/repo"
	"mangoduck/internal/telegram/shared"
	"mangoduck/internal/telegram/tgerr"
)

const (
	toggleChatStatusButtonUnique = "toggle_chat_status"
	chatsPageButtonUnique        = "chats_page"
	chatsPageSize                = 5
	adminPanelPrivateChatText    = "Open this admin panel in a private chat with the bot."
)

type Repository interface {
	Create(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error)
	GetByTGID(ctx context.Context, tgID int64) (*repo.Chat, error)
	List(ctx context.Context) ([]*repo.Chat, error)
	UpdateProfile(ctx context.Context, tgID int64, title string, username string, chatType string) error
	UpdateStatus(ctx context.Context, tgID int64, status repo.ChatStatus) error
}

type ApprovalNotifier interface {
	NotifyApproved(chatRecord *repo.Chat) error
}

type panelState struct {
	Page int
}

type stats struct {
	Total      int
	Active     int
	Inactive   int
	Private    int
	Group      int
	Supergroup int
	Channel    int
}

func ToggleChatStatusButtonUnique() string {
	return toggleChatStatusButtonUnique
}

func ChatsPageButtonUnique() string {
	return chatsPageButtonUnique
}

func Chats(cfg config.Config, chatsRepo Repository) func(tele.Context) error {
	return func(c tele.Context) error {
		_, err := RequireActiveAdmin(c, cfg)
		if err != nil {
			if errors.Is(err, shared.ErrResponseHandled) {
				return nil
			}

			return err
		}

		err = RequirePrivateChat(c, adminPanelPrivateChatText)
		if err != nil {
			if errors.Is(err, shared.ErrResponseHandled) {
				return nil
			}

			return err
		}

		text, markup, err := buildChatsPanel(context.Background(), chatsRepo, defaultPanelState())
		if err != nil {
			return err
		}

		return c.Send(text, markup)
	}
}

func ToggleChatStatus(cfg config.Config, chatsRepo Repository, approvalNotifier ApprovalNotifier) func(tele.Context) error {
	return func(c tele.Context) error {
		handled, err := ensureAdminPanelAccess(c, cfg)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}

		tgID, status, currentPanelState, err := parseToggleChatStatusArgs(c)
		if err != nil {
			return err
		}

		targetChat, wasInactive, err := updateTargetChatStatus(chatsRepo, tgID, status)
		if err != nil {
			if errors.Is(err, repo.ErrChatNotFound) {
				return c.Respond(&tele.CallbackResponse{Text: "chat not found", ShowAlert: true})
			}

			return err
		}

		err = notifyChatApproved(approvalNotifier, targetChat, wasInactive, status)
		if err != nil {
			return err
		}

		return editChatsPanel(c, chatsRepo, currentPanelState)
	}
}

func updateTargetChatStatus(chatsRepo Repository, tgID int64, status repo.ChatStatus) (*repo.Chat, bool, error) {
	targetChat, err := chatsRepo.GetByTGID(context.Background(), tgID)
	if err != nil {
		return nil, false, fmt.Errorf("getting target chat: %w", err)
	}

	wasInactive := targetChat.Status == repo.ChatStatusInactive

	err = chatsRepo.UpdateStatus(context.Background(), tgID, status)
	if err != nil {
		return nil, false, fmt.Errorf("updating chat status: %w", err)
	}
	targetChat.Status = status

	return targetChat, wasInactive, nil
}

func notifyChatApproved(approvalNotifier ApprovalNotifier, targetChat *repo.Chat, wasInactive bool, status repo.ChatStatus) error {
	if approvalNotifier == nil || targetChat == nil {
		return nil
	}
	if !wasInactive || status != repo.ChatStatusActive {
		return nil
	}

	err := approvalNotifier.NotifyApproved(targetChat)
	if err != nil {
		return fmt.Errorf("sending approval notification: %w", err)
	}

	return nil
}

func ensureAdminPanelAccess(c tele.Context, cfg config.Config) (bool, error) {
	_, err := RequireActiveAdmin(c, cfg)
	if err != nil {
		if errors.Is(err, shared.ErrResponseHandled) {
			return true, nil
		}

		return false, err
	}

	err = RequirePrivateChat(c, adminPanelPrivateChatText)
	if err != nil {
		if errors.Is(err, shared.ErrResponseHandled) {
			return true, nil
		}

		return false, err
	}

	return false, nil
}

func parseToggleChatStatusArgs(c tele.Context) (int64, repo.ChatStatus, panelState, error) {
	args := c.Args()
	if len(args) != 3 {
		return 0, "", panelState{}, respondCallbackError(c, "invalid callback")
	}

	tgID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, "", panelState{}, respondCallbackError(c, "invalid chat")
	}

	status := repo.ChatStatus(args[1])
	if status != repo.ChatStatusActive && status != repo.ChatStatusInactive {
		return 0, "", panelState{}, respondCallbackError(c, "invalid status")
	}

	currentPanelState, err := parsePanelState(args[2])
	if err != nil {
		return 0, "", panelState{}, respondCallbackError(c, "invalid page")
	}

	return tgID, status, currentPanelState, nil
}

func respondCallbackError(c tele.Context, text string) error {
	return c.Respond(&tele.CallbackResponse{Text: text, ShowAlert: true})
}

func ChatsPage(cfg config.Config, chatsRepo Repository) func(tele.Context) error {
	return func(c tele.Context) error {
		_, err := RequireActiveAdmin(c, cfg)
		if err != nil {
			if errors.Is(err, shared.ErrResponseHandled) {
				return nil
			}

			return err
		}

		err = RequirePrivateChat(c, adminPanelPrivateChatText)
		if err != nil {
			if errors.Is(err, shared.ErrResponseHandled) {
				return nil
			}

			return err
		}

		args := c.Args()
		if len(args) != 1 {
			return c.Respond(&tele.CallbackResponse{Text: "invalid callback", ShowAlert: true})
		}

		currentPanelState, err := parsePanelState(args[0])
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "invalid page", ShowAlert: true})
		}

		return editChatsPanel(c, chatsRepo, currentPanelState)
	}
}

func WaitForChatApprovalMessage(tgID int64) string {
	return fmt.Sprintf("wait for chat approval\nChat ID: %d", tgID)
}

func EnsureCurrentChat(c tele.Context, chatsRepo Repository) (*repo.Chat, bool, error) {
	chat, err := currentChat(c)
	if err != nil {
		return nil, false, err
	}

	title, username, chatType := resolveChatProfile(c, chat)
	ctx := context.Background()
	justCreated := false

	currentChatRecord, err := chatsRepo.GetByTGID(ctx, chat.ID)
	if err != nil {
		if !errors.Is(err, repo.ErrChatNotFound) {
			return nil, false, fmt.Errorf("getting chat by tg id: %w", err)
		}

		currentChatRecord, err = chatsRepo.Create(ctx, chat.ID, title, username, chatType, repo.ChatStatusInactive)
		if err != nil {
			return nil, false, fmt.Errorf("creating chat: %w", err)
		}

		justCreated = true
		return currentChatRecord, justCreated, nil
	}

	if currentChatRecord.Title != title || currentChatRecord.Username != username || currentChatRecord.Type != chatType {
		err = chatsRepo.UpdateProfile(ctx, chat.ID, title, username, chatType)
		if err != nil {
			return nil, false, fmt.Errorf("updating chat profile: %w", err)
		}

		currentChatRecord.Title = title
		currentChatRecord.Username = username
		currentChatRecord.Type = chatType
	}

	return currentChatRecord, justCreated, nil
}

func RequireResolvedActiveChat(c tele.Context, currentChatRecord *repo.Chat) (*repo.Chat, error) {
	if currentChatRecord == nil {
		return nil, errors.New("current chat is nil")
	}

	if currentChatRecord.Status != repo.ChatStatusActive {
		err := c.Send(WaitForChatApprovalMessage(currentChatRecord.TGID))
		if err != nil {
			return nil, err
		}

		return nil, shared.ErrResponseHandled
	}

	return currentChatRecord, nil
}

func RequireActiveAdmin(c tele.Context, cfg config.Config) (*tele.User, error) {
	sender := c.Sender()
	if sender == nil {
		return nil, errors.New("sender is nil")
	}

	if !cfg.IsAdminTGID(sender.ID) {
		err := c.Send("forbidden")
		if err != nil {
			return nil, err
		}

		return nil, shared.ErrResponseHandled
	}

	return sender, nil
}

func RequirePrivateChat(c tele.Context, message string) error {
	chat, err := currentChat(c)
	if err != nil {
		return err
	}

	if normalizeChatType(string(chat.Type)) == "private" {
		return nil
	}

	err = c.Send(strings.TrimSpace(message))
	if err != nil {
		return err
	}

	return shared.ErrResponseHandled
}

func editChatsPanel(c tele.Context, chatsRepo Repository, currentPanelState panelState) error {
	text, markup, err := buildChatsPanel(context.Background(), chatsRepo, currentPanelState)
	if err != nil {
		return err
	}

	err = c.Edit(text, markup)
	if err != nil {
		if errors.Is(tgerr.Normalize(err), tgerr.ErrMessageNotModified) {
			return c.Respond()
		}

		return fmt.Errorf("editing chats panel: %w", err)
	}

	return c.Respond()
}

func buildChatsPanel(ctx context.Context, chatsRepo Repository, currentPanelState panelState) (string, *tele.ReplyMarkup, error) {
	chatList, err := chatsRepo.List(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("listing chats: %w", err)
	}

	_, currentStats := filterChats(chatList)
	totalPages := calcTotalPages(len(chatList))
	currentPanelState.Page = normalizePage(currentPanelState.Page, totalPages)

	pageChats := sliceChatsPage(chatList, currentPanelState.Page)
	text := renderChatsPanelText(pageChats, currentStats, currentPanelState, totalPages)
	markup := renderChatsPanelMarkup(pageChats, currentPanelState, totalPages)

	return text, markup, nil
}

func renderChatsPanelText(chatList []*repo.Chat, currentStats stats, currentPanelState panelState, totalPages int) string {
	lines := make([]string, 0, len(chatList)*3+8)
	lines = append(lines, "Chats Control Panel")
	lines = append(lines, fmt.Sprintf("Summary: total=%d | active=%d | inactive=%d | private=%d | group=%d | supergroup=%d | channel=%d", currentStats.Total, currentStats.Active, currentStats.Inactive, currentStats.Private, currentStats.Group, currentStats.Supergroup, currentStats.Channel))
	lines = append(lines, fmt.Sprintf("Page: %d/%d", currentPanelState.Page, totalPages))
	lines = append(lines, "")

	if len(chatList) == 0 {
		lines = append(lines, "No chats.")
		return strings.Join(lines, "\n")
	}

	for idx, chat := range chatList {
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, displayChatName(chat)))
		lines = append(lines, fmt.Sprintf("   tg_id=%d | type=%s | status=%s", chat.TGID, chat.Type, chat.Status))
	}

	return strings.Join(lines, "\n")
}

func renderChatsPanelMarkup(chatList []*repo.Chat, currentPanelState panelState, totalPages int) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0)

	for _, chat := range chatList {
		nextStatus := repo.ChatStatusActive
		buttonText := "Activate"
		if chat.Status == repo.ChatStatusActive {
			nextStatus = repo.ChatStatusInactive
			buttonText = "Deactivate"
		}

		rows = append(rows, markup.Row(
			markup.Data(buttonText+" "+displayChatName(chat), toggleChatStatusButtonUnique, strconv.FormatInt(chat.TGID, 10), string(nextStatus), strconv.Itoa(currentPanelState.Page)),
		))
	}

	navButtons := make([]tele.Btn, 0, 2)
	if currentPanelState.Page > 1 {
		navButtons = append(navButtons, markup.Data("Prev", chatsPageButtonUnique, strconv.Itoa(currentPanelState.Page-1)))
	}

	if currentPanelState.Page < totalPages {
		navButtons = append(navButtons, markup.Data("Next", chatsPageButtonUnique, strconv.Itoa(currentPanelState.Page+1)))
	}

	if len(navButtons) > 0 {
		rows = append(rows, markup.Row(navButtons...))
	}

	markup.Inline(rows...)

	return markup
}

func filterChats(chatList []*repo.Chat) ([]*repo.Chat, stats) {
	var currentStats stats

	for _, chat := range chatList {
		currentStats.Total++
		if chat.Status == repo.ChatStatusActive {
			currentStats.Active++
		}
		if chat.Status == repo.ChatStatusInactive {
			currentStats.Inactive++
		}

		switch normalizeChatType(chat.Type) {
		case "private":
			currentStats.Private++
		case "group":
			currentStats.Group++
		case "supergroup":
			currentStats.Supergroup++
		case "channel":
			currentStats.Channel++
		}
	}

	return chatList, currentStats
}

func defaultPanelState() panelState {
	var state panelState
	state.Page = 1
	return state
}

func parsePanelState(pageRaw string) (panelState, error) {
	page, err := strconv.Atoi(pageRaw)
	if err != nil {
		return panelState{}, fmt.Errorf("parsing page: %w", err)
	}

	var state panelState
	state.Page = page
	return state, nil
}

func calcTotalPages(totalChats int) int {
	if totalChats == 0 {
		return 1
	}

	pages := totalChats / chatsPageSize
	if totalChats%chatsPageSize != 0 {
		pages++
	}

	return pages
}

func normalizePage(page, totalPages int) int {
	if page < 1 {
		return 1
	}
	if page > totalPages {
		return totalPages
	}

	return page
}

func sliceChatsPage(chatList []*repo.Chat, page int) []*repo.Chat {
	start := (page - 1) * chatsPageSize
	if start >= len(chatList) {
		return []*repo.Chat{}
	}

	end := start + chatsPageSize
	if end > len(chatList) {
		end = len(chatList)
	}

	return chatList[start:end]
}

func displayChatName(chat *repo.Chat) string {
	title := strings.TrimSpace(chat.Title)
	if title != "" {
		return title
	}

	username := strings.TrimSpace(chat.Username)
	if username != "" {
		return "@" + username
	}

	return strconv.FormatInt(chat.TGID, 10)
}

func resolveChatProfile(c tele.Context, chat *tele.Chat) (string, string, string) {
	chatType := normalizeChatType(string(chat.Type))
	if chatType == "private" {
		sender := c.Sender()
		if sender == nil {
			return "", "", chatType
		}

		return "", strings.TrimSpace(sender.Username), chatType
	}

	return strings.TrimSpace(chat.Title), "", chatType
}

func currentChat(c tele.Context) (*tele.Chat, error) {
	chat := c.Chat()
	if chat == nil {
		return nil, errors.New("chat is nil")
	}

	return chat, nil
}

func normalizeChatType(chatType string) string {
	return strings.ToLower(strings.TrimSpace(chatType))
}
