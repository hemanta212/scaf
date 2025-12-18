package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rlch/neogo"
	"github.com/rlch/scaf/cmd/crud-tui/service"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DDDDDD"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			Padding(1, 0)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	authorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9B9B9B")).
			Italic(true)

	replyIndent = lipgloss.NewStyle().
			PaddingLeft(2)
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
}

type (
	postsLoadedMsg    []*service.Post
	commentsLoadedMsg []*service.Comment
	repliesLoadedMsg  []*service.Comment
	postCreatedMsg    *service.Post
	commentCreatedMsg *service.Comment
	replyCreatedMsg   *service.Comment
	errMsg            error
)

func initialModel(ctx context.Context, db neogo.Driver) model {
	return model{
		ctx:         ctx,
		db:          db,
		postSvc:     service.NewPostService(db),
		commentSvc:  service.NewCommentService(db),
		currentView: viewPosts,
		currentUser: 1,
	}
}

func (m model) Init() tea.Cmd {
	return m.loadPosts()
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
		m.message = fmt.Sprintf("Created post: %s", msg.Title)
		m.inputMode = inputNone
		m.clearInput()
		return m, m.loadPosts()

	case commentCreatedMsg:
		m.message = "Created comment"
		m.inputMode = inputNone
		m.clearInput()
		if m.selectedPost != nil {
			return m, m.loadComments(m.selectedPost.ID)
		}

	case replyCreatedMsg:
		m.message = "Created reply"
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
		return "Loading..."
	}

	var b strings.Builder

	// Header
	var title string
	switch m.currentView {
	case viewPosts:
		title = " Posts "
	case viewComments:
		if m.selectedPost != nil {
			title = fmt.Sprintf(" Comments on: %s ", truncate(m.selectedPost.Title, 30))
		}
	case viewReplies:
		if m.selectedComment != nil {
			title = fmt.Sprintf(" Replies to: %s ", truncate(m.selectedComment.Text, 30))
		}
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	// Input mode
	if m.inputMode != inputNone {
		b.WriteString(m.renderInput())
		b.WriteString("\n")
	} else {
		// List
		b.WriteString(m.renderList())
		b.WriteString("\n")
	}

	// Status
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	} else if m.message != "" {
		b.WriteString(successStyle.Render(m.message))
		b.WriteString("\n")
	}

	// Help
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m model) renderInput() string {
	var b strings.Builder

	switch m.inputMode {
	case inputNewPost:
		titleLabel := "Title: "
		contentLabel := "Content: "
		if m.inputField == 0 {
			titleLabel = selectedStyle.Render("> Title: ")
		} else {
			contentLabel = selectedStyle.Render("> Content: ")
		}
		b.WriteString(borderStyle.Render(
			titleLabel + m.inputTitle + "\n" +
				contentLabel + m.inputContent,
		))
	case inputNewComment:
		b.WriteString(borderStyle.Render(
			selectedStyle.Render("> Comment: ") + m.inputBuffer,
		))
	case inputNewReply:
		b.WriteString(borderStyle.Render(
			selectedStyle.Render("> Reply: ") + m.inputBuffer,
		))
	}

	return b.String()
}

func (m model) renderList() string {
	var b strings.Builder

	switch m.currentView {
	case viewPosts:
		if len(m.posts) == 0 {
			b.WriteString(dimStyle.Render("No posts yet. Press 'n' to create one."))
		}
		for i, p := range m.posts {
			line := fmt.Sprintf("%s %s", p.Title, authorStyle.Render("by "+p.AuthorName))
			if i == m.selectedIdx {
				b.WriteString(selectedStyle.Render("> " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}

	case viewComments:
		if len(m.comments) == 0 {
			b.WriteString(dimStyle.Render("No comments yet. Press 'n' to add one."))
		}
		for i, c := range m.comments {
			line := fmt.Sprintf("%s %s", c.Text, authorStyle.Render("- "+c.AuthorName))
			if i == m.selectedIdx {
				b.WriteString(selectedStyle.Render("> " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}

	case viewReplies:
		if len(m.replies) == 0 {
			b.WriteString(dimStyle.Render("No replies yet. Press 'n' to add one."))
		}
		for i, r := range m.replies {
			line := fmt.Sprintf("%s %s", r.Text, authorStyle.Render("- "+r.AuthorName))
			if i == m.selectedIdx {
				b.WriteString(replyIndent.Render(selectedStyle.Render("> " + line)))
			} else {
				b.WriteString(replyIndent.Render(normalStyle.Render("  " + line)))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m model) renderHelp() string {
	var help string
	if m.inputMode != inputNone {
		if m.inputMode == inputNewPost {
			help = "tab: switch field | enter: submit | esc: cancel"
		} else {
			help = "enter: submit | esc: cancel"
		}
	} else {
		switch m.currentView {
		case viewPosts:
			help = "j/k: navigate | enter: view comments | n: new post | r: refresh | q: quit"
		case viewComments:
			help = "j/k: navigate | enter: view replies | n: new comment | backspace: back | r: refresh | q: quit"
		case viewReplies:
			help = "j/k: navigate | n: new reply | backspace: back | r: refresh | q: quit"
		}
	}
	return helpStyle.Render(help)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func main() {
	uri := os.Getenv("NEO4J_URI")
	if uri == "" {
		uri = "bolt://localhost:7689"
	}
	user := os.Getenv("NEO4J_USER")
	if user == "" {
		user = "neo4j"
	}
	pass := os.Getenv("NEO4J_PASS")
	if pass == "" {
		pass = "password"
	}

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
