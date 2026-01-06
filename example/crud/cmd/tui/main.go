package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rlch/neogo"
	"github.com/rlch/scaf"
	service "github.com/rlch/scaf/example/crud/internal"
)

// Twitter/X Dark Mode Color Palette
var (
	// Core colors
	colorBlack     = lipgloss.Color("#000000")
	colorDarkBg    = lipgloss.Color("#15202B") // Twitter dark blue bg
	colorDarkerBg  = lipgloss.Color("#192734") // Slightly lighter for cards
	colorBorder    = lipgloss.Color("#38444D") // Muted border
	colorDimText   = lipgloss.Color("#8899A6") // Secondary text
	colorText      = lipgloss.Color("#E7E9EA") // Primary text
	colorWhite     = lipgloss.Color("#FFFFFF")
	colorBlue      = lipgloss.Color("#1D9BF0") // Twitter blue
	colorGreen     = lipgloss.Color("#00BA7C") // Success green
	colorRed       = lipgloss.Color("#F4212E") // Error red
	colorLightBlue = lipgloss.Color("#8ECDF8") // Hover/selected blue
	colorReplyLine = lipgloss.Color("#2F3336") // Reply thread line
)

// Styles
var (
	// App container - full dark bg
	appStyle = lipgloss.NewStyle().
			Background(colorDarkBg)

	// Header bar style
	headerStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true).
			Padding(1, 2).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colorBorder)

	// Tweet/Post card style
	postCardStyle = lipgloss.NewStyle().
			Padding(1, 2).
			MarginBottom(0).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colorBorder)

	// Selected post highlight
	selectedPostStyle = lipgloss.NewStyle().
				Padding(1, 2).
				MarginBottom(0).
				Background(colorDarkerBg).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(colorBlue).
				BorderLeft(true)

	// Username/handle style
	usernameStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true)

	handleStyle = lipgloss.NewStyle().
			Foreground(colorDimText)

	// Content text
	contentStyle = lipgloss.NewStyle().
			Foreground(colorText).
			MarginTop(1)

	// Selected content with blue tint
	selectedContentStyle = lipgloss.NewStyle().
				Foreground(colorLightBlue).
				MarginTop(1)

	// Timestamp style
	timestampStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			MarginLeft(1)

	// Action bar (replies, likes, etc)
	actionBarStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			MarginTop(1)

	// Reply thread indicator
	replyLineStyle = lipgloss.NewStyle().
			Foreground(colorReplyLine).
			SetString("│").
			PaddingLeft(1)

	// Input composer box
	composerStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			MarginBottom(1)

	composerLabelStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true)

	composerInputStyle = lipgloss.NewStyle().
				Foreground(colorText)

	// Tab/navigation indicator
	tabActiveStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			Underline(true).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDimText).
				Padding(0, 1)

	// Status messages
	errorMsgStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true).
			Padding(0, 2)

	successMsgStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true).
			Padding(0, 2)

	// Help footer
	helpStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Padding(1, 2).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(colorBorder)

	// Empty state
	emptyStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Italic(true).
			Padding(2, 2)

	// Cursor/caret for input
	cursorStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			SetString("▎")
)

type view int

const (
	viewPosts view = iota
	viewComments
	viewReplies
	viewInput
)

type inputMode int

const (
	inputNone inputMode = iota
	inputNewPost
	inputNewComment
	inputNewReply
)

type model struct {
	ctx             context.Context
	db              neogo.Driver
	postSvc         *service.PostService
	commentSvc      *service.CommentService
	currentView     view
	currentUser     int
	posts           []*service.Post
	comments        []*service.Comment
	replies         []*service.Comment
	selectedIdx     int
	selectedPost    *service.Post
	selectedComment *service.Comment
	width           int
	height          int
	err             error
	message         string
	inputMode       inputMode
	inputBuffer     string
	inputField      int
	inputTitle      string
	inputContent    string
	cursorBlink     bool
}

type (
	postsLoadedMsg    []*service.Post
	commentsLoadedMsg []*service.Comment
	repliesLoadedMsg  []*service.Comment
	postCreatedMsg    *service.Post
	commentCreatedMsg *service.Comment
	replyCreatedMsg   *service.Comment
	errMsg            error
	cursorBlinkMsg    struct{}
)

func initialModel(ctx context.Context, db neogo.Driver) model {
	return model{
		ctx:         ctx,
		db:          db,
		postSvc:     service.NewPostService(db),
		commentSvc:  service.NewCommentService(db),
		currentView: viewPosts,
		currentUser: 1,
		cursorBlink: true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadPosts(), m.blinkCursor())
}

func (m model) blinkCursor() tea.Cmd {
	return tea.Tick(time.Millisecond*530, func(t time.Time) tea.Msg {
		return cursorBlinkMsg{}
	})
}

func (m model) loadPosts() tea.Cmd {
	return func() tea.Msg {
		posts, err := m.postSvc.List(m.ctx)
		if err != nil {
			return errMsg(err)
		}
		return postsLoadedMsg(posts)
	}
}

func (m model) loadComments(postID int) tea.Cmd {
	return func() tea.Msg {
		comments, err := m.commentSvc.GetByPost(m.ctx, postID)
		if err != nil {
			return errMsg(err)
		}
		return commentsLoadedMsg(comments)
	}
}

func (m model) loadReplies(commentID int) tea.Cmd {
	return func() tea.Msg {
		replies, err := m.commentSvc.GetReplies(m.ctx, commentID)
		if err != nil {
			return errMsg(err)
		}
		return repliesLoadedMsg(replies)
	}
}

func (m model) createPost(title, content string) tea.Cmd {
	return func() tea.Msg {
		post, err := m.postSvc.Create(m.ctx, title, content, m.currentUser)
		if err != nil {
			return errMsg(err)
		}
		return postCreatedMsg(post)
	}
}

func (m model) createComment(text string, postID int) tea.Cmd {
	return func() tea.Msg {
		comment, err := m.commentSvc.Create(m.ctx, text, m.currentUser, postID)
		if err != nil {
			return errMsg(err)
		}
		return commentCreatedMsg(comment)
	}
}

func (m model) createReply(text string, parentID int) tea.Cmd {
	return func() tea.Msg {
		reply, err := m.commentSvc.Reply(m.ctx, text, m.currentUser, parentID)
		if err != nil {
			return errMsg(err)
		}
		return replyCreatedMsg(reply)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case cursorBlinkMsg:
		m.cursorBlink = !m.cursorBlink
		return m, m.blinkCursor()

	case tea.KeyMsg:
		if m.inputMode != inputNone {
			return m.handleInputKey(msg)
		}
		return m.handleNavigationKey(msg)

	case postsLoadedMsg:
		m.posts = msg
		m.selectedIdx = 0
		m.message = ""
		m.err = nil

	case commentsLoadedMsg:
		m.comments = msg
		m.selectedIdx = 0
		m.message = ""
		m.err = nil

	case repliesLoadedMsg:
		m.replies = msg
		m.selectedIdx = 0
		m.message = ""
		m.err = nil

	case postCreatedMsg:
		m.message = fmt.Sprintf("Posted: %s", msg.Title)
		m.inputMode = inputNone
		m.clearInput()
		return m, m.loadPosts()

	case commentCreatedMsg:
		m.message = "Reply posted"
		m.inputMode = inputNone
		m.clearInput()
		if m.selectedPost != nil {
			return m, m.loadComments(m.selectedPost.ID)
		}

	case replyCreatedMsg:
		m.message = "Reply posted"
		m.inputMode = inputNone
		m.clearInput()
		if m.selectedComment != nil {
			return m, m.loadReplies(m.selectedComment.ID)
		}

	case errMsg:
		m.err = msg
	}

	return m, nil
}

func (m *model) clearInput() {
	m.inputBuffer = ""
	m.inputTitle = ""
	m.inputContent = ""
	m.inputField = 0
}

func (m model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.inputMode = inputNone
		m.clearInput()
		return m, nil

	case "tab":
		if m.inputMode == inputNewPost {
			m.inputField = (m.inputField + 1) % 2
		}
		return m, nil

	case "enter":
		switch m.inputMode {
		case inputNewPost:
			if m.inputTitle != "" {
				return m, m.createPost(m.inputTitle, m.inputContent)
			}
		case inputNewComment:
			if m.inputBuffer != "" && m.selectedPost != nil {
				return m, m.createComment(m.inputBuffer, m.selectedPost.ID)
			}
		case inputNewReply:
			if m.inputBuffer != "" && m.selectedComment != nil {
				return m, m.createReply(m.inputBuffer, m.selectedComment.ID)
			}
		}
		return m, nil

	case "backspace":
		if m.inputMode == inputNewPost {
			if m.inputField == 0 && len(m.inputTitle) > 0 {
				m.inputTitle = m.inputTitle[:len(m.inputTitle)-1]
			} else if m.inputField == 1 && len(m.inputContent) > 0 {
				m.inputContent = m.inputContent[:len(m.inputContent)-1]
			}
		} else {
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
		}
		return m, nil

	default:
		char := msg.String()
		if msg.Type == tea.KeyRunes {
			char = string(msg.Runes)
		} else if char == "space" {
			char = " "
		} else if len(char) != 1 {
			return m, nil
		}
		if m.inputMode == inputNewPost {
			if m.inputField == 0 {
				m.inputTitle += char
			} else {
				m.inputContent += char
			}
		} else {
			m.inputBuffer += char
		}
	}

	return m, nil
}

func (m model) handleNavigationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}

	case "down", "j":
		maxIdx := m.getMaxIndex()
		if m.selectedIdx < maxIdx-1 {
			m.selectedIdx++
		}

	case "n":
		switch m.currentView {
		case viewPosts:
			m.inputMode = inputNewPost
		case viewComments:
			if m.selectedPost != nil {
				m.inputMode = inputNewComment
			}
		case viewReplies:
			if m.selectedComment != nil {
				m.inputMode = inputNewReply
			}
		}
		m.clearInput()
		return m, nil

	case "enter", "l":
		switch m.currentView {
		case viewPosts:
			if len(m.posts) > 0 && m.selectedIdx < len(m.posts) {
				m.selectedPost = m.posts[m.selectedIdx]
				m.currentView = viewComments
				m.selectedIdx = 0
				return m, m.loadComments(m.selectedPost.ID)
			}
		case viewComments:
			if len(m.comments) > 0 && m.selectedIdx < len(m.comments) {
				m.selectedComment = m.comments[m.selectedIdx]
				m.currentView = viewReplies
				m.selectedIdx = 0
				return m, m.loadReplies(m.selectedComment.ID)
			}
		}
		return m, nil

	case "backspace", "h", "esc":
		switch m.currentView {
		case viewComments:
			m.currentView = viewPosts
			m.selectedPost = nil
			m.selectedIdx = 0
		case viewReplies:
			m.currentView = viewComments
			m.selectedComment = nil
			m.selectedIdx = 0
		}
		return m, nil

	case "r":
		switch m.currentView {
		case viewPosts:
			return m, m.loadPosts()
		case viewComments:
			if m.selectedPost != nil {
				return m, m.loadComments(m.selectedPost.ID)
			}
		case viewReplies:
			if m.selectedComment != nil {
				return m, m.loadReplies(m.selectedComment.ID)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m model) getMaxIndex() int {
	switch m.currentView {
	case viewPosts:
		return len(m.posts)
	case viewComments:
		return len(m.comments)
	case viewReplies:
		return len(m.replies)
	}
	return 0
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	var sections []string

	// Header with breadcrumb navigation
	sections = append(sections, m.renderHeader())

	// Composer (if in input mode)
	if m.inputMode != inputNone {
		sections = append(sections, m.renderComposer())
	}

	// Status messages
	if m.err != nil {
		sections = append(sections, errorMsgStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	} else if m.message != "" {
		sections = append(sections, successMsgStyle.Render(m.message))
	}

	// Content feed
	sections = append(sections, m.renderFeed())

	// Help footer
	sections = append(sections, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m model) renderHeader() string {
	var tabs []string

	// Breadcrumb-style tabs
	postsTab := "Home"
	if m.currentView == viewPosts {
		tabs = append(tabs, tabActiveStyle.Render(postsTab))
	} else {
		tabs = append(tabs, tabInactiveStyle.Render(postsTab))
	}

	if m.currentView == viewComments || m.currentView == viewReplies {
		tabs = append(tabs, handleStyle.Render(" / "))
		postTitle := "Post"
		if m.selectedPost != nil {
			postTitle = truncate(m.selectedPost.Title, 20)
		}
		if m.currentView == viewComments {
			tabs = append(tabs, tabActiveStyle.Render(postTitle))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(postTitle))
		}
	}

	if m.currentView == viewReplies {
		tabs = append(tabs, handleStyle.Render(" / "))
		tabs = append(tabs, tabActiveStyle.Render("Thread"))
	}

	header := lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
	return headerStyle.Width(m.width).Render(header)
}

func (m model) renderComposer() string {
	var content string
	cursor := ""
	if m.cursorBlink {
		cursor = cursorStyle.String()
	}

	switch m.inputMode {
	case inputNewPost:
		titleLabel := "What's happening?"
		contentLabel := "Add more details..."
		titleInput := m.inputTitle
		contentInput := m.inputContent

		if m.inputField == 0 {
			titleLabel = composerLabelStyle.Render(titleLabel)
			titleInput = composerInputStyle.Render(titleInput) + cursor
			contentLabel = handleStyle.Render(contentLabel)
			contentInput = handleStyle.Render(contentInput)
		} else {
			titleLabel = handleStyle.Render(titleLabel)
			titleInput = handleStyle.Render(titleInput)
			contentLabel = composerLabelStyle.Render(contentLabel)
			contentInput = composerInputStyle.Render(contentInput) + cursor
		}

		content = lipgloss.JoinVertical(lipgloss.Left,
			titleLabel,
			titleInput,
			"",
			contentLabel,
			contentInput,
		)

	case inputNewComment:
		label := composerLabelStyle.Render("Post your reply")
		input := composerInputStyle.Render(m.inputBuffer) + cursor
		content = lipgloss.JoinVertical(lipgloss.Left, label, input)

	case inputNewReply:
		label := composerLabelStyle.Render("Reply to thread")
		input := composerInputStyle.Render(m.inputBuffer) + cursor
		content = lipgloss.JoinVertical(lipgloss.Left, label, input)
	}

	return composerStyle.Width(m.width - 4).Render(content)
}

func (m model) renderFeed() string {
	var items []string

	switch m.currentView {
	case viewPosts:
		if len(m.posts) == 0 {
			return emptyStyle.Render("Nothing to see yet. Press 'n' to post something.")
		}
		for i, p := range m.posts {
			items = append(items, m.renderPostCard(p, i == m.selectedIdx))
		}

	case viewComments:
		// Show the parent post first
		if m.selectedPost != nil {
			items = append(items, m.renderPostCard(m.selectedPost, false))
		}
		if len(m.comments) == 0 {
			items = append(items, emptyStyle.Render("No replies yet. Be the first to reply."))
		}
		for i, c := range m.comments {
			items = append(items, m.renderCommentCard(c, i == m.selectedIdx, false))
		}

	case viewReplies:
		// Show the parent comment
		if m.selectedComment != nil {
			items = append(items, m.renderCommentCard(m.selectedComment, false, false))
		}
		if len(m.replies) == 0 {
			items = append(items, emptyStyle.Render("No replies in this thread yet."))
		}
		for i, r := range m.replies {
			items = append(items, m.renderCommentCard(r, i == m.selectedIdx, true))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, items...)
}

func (m model) renderPostCard(p *service.Post, selected bool) string {
	// Username and handle
	name := usernameStyle.Render(p.AuthorName)
	handle := handleStyle.Render(" @" + strings.ToLower(strings.ReplaceAll(p.AuthorName, " ", "")))
	timestamp := timestampStyle.Render("now")

	userLine := lipgloss.JoinHorizontal(lipgloss.Left, name, handle, timestamp)

	// Post content
	title := p.Title
	body := ""
	if p.Content != "" {
		body = "\n" + handleStyle.Render(p.Content)
	}

	var contentText string
	if selected {
		contentText = selectedContentStyle.Render(title) + body
	} else {
		contentText = contentStyle.Render(title) + body
	}

	// Action bar
	actions := actionBarStyle.Render("    Reply     Repost     Like     Share")

	card := lipgloss.JoinVertical(lipgloss.Left, userLine, contentText, actions)

	if selected {
		return selectedPostStyle.Width(m.width).Render(card)
	}
	return postCardStyle.Width(m.width).Render(card)
}

func (m model) renderCommentCard(c *service.Comment, selected bool, isReply bool) string {
	// Thread line for replies
	prefix := ""
	if isReply {
		prefix = replyLineStyle.String() + " "
	}

	// Username and handle
	name := usernameStyle.Render(c.AuthorName)
	handle := handleStyle.Render(" @" + strings.ToLower(strings.ReplaceAll(c.AuthorName, " ", "")))
	timestamp := timestampStyle.Render("now")

	userLine := prefix + lipgloss.JoinHorizontal(lipgloss.Left, name, handle, timestamp)

	// Comment content
	var contentText string
	if selected {
		contentText = prefix + selectedContentStyle.Render(c.Text)
	} else {
		contentText = prefix + contentStyle.Render(c.Text)
	}

	// Action bar
	actions := prefix + actionBarStyle.Render("    Reply     Repost     Like")

	card := lipgloss.JoinVertical(lipgloss.Left, userLine, contentText, actions)

	if selected {
		return selectedPostStyle.Width(m.width).Render(card)
	}
	return postCardStyle.Width(m.width).Render(card)
}

func (m model) renderHelp() string {
	var help string
	if m.inputMode != inputNone {
		if m.inputMode == inputNewPost {
			help = "TAB switch field  ENTER post  ESC cancel"
		} else {
			help = "ENTER post  ESC cancel"
		}
	} else {
		switch m.currentView {
		case viewPosts:
			help = "j/k navigate  ENTER open  n new post  r refresh  q quit"
		case viewComments:
			help = "j/k navigate  ENTER thread  n reply  h back  r refresh  q quit"
		case viewReplies:
			help = "j/k navigate  n reply  h back  r refresh  q quit"
		}
	}
	return helpStyle.Width(m.width).Render(help)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func main() {
	uri, user, pass := loadNeo4jConfig()

	ctx := context.Background()
	db, err := neogo.New(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Neo4j: %v\n", err)
		os.Exit(1)
	}
	defer db.DB().Close(ctx)

	p := tea.NewProgram(
		initialModel(ctx, db),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func loadNeo4jConfig() (uri, user, pass string) {
	configPath, err := scaf.FindConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: .scaf.yaml not found: %v\n", err)
		os.Exit(1)
	}
	cfg, err := scaf.LoadConfigFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
		os.Exit(1)
	}
	if cfg.Neo4j == nil {
		fmt.Fprintf(os.Stderr, "error: neo4j config not found in .scaf.yaml\n")
		os.Exit(1)
	}
	return cfg.Neo4j.URI, cfg.Neo4j.Username, cfg.Neo4j.Password
}
