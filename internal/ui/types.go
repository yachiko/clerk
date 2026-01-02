package ui

import "github.com/yachiko/clerk/internal/cache"

// ViewMode represents the current view mode
type ViewMode int

const (
	ViewModeList ViewMode = iota
	ViewModeTree
	ViewModeDescribe
)

// State represents the UI state
type State struct {
	// Data
	Entries       []cache.CacheEntry
	FilteredItems []cache.CacheEntry

	// View state
	Mode          ViewMode
	SelectedIndex int
	ScrollOffset  int

	// Search
	SearchQuery  string
	SearchActive bool

	// Describe view state
	DescribeEntry       *cache.CacheEntry
	DescribeParamName   string // Track parameter name for lazy loading
	DescribeValue       string
	DescribeMasked      bool
	DescribeHistory     []HistoryEntry
	HistoryIndex        int
	HistoryScrollOffset int
	ValueScrollOffset   int

	// Tree view state
	TreeNodes     []TreeNode
	ExpandedPaths map[string]bool

	// Confirmation dialog
	Confirm ConfirmState

	// Window dimensions
	Width  int
	Height int

	// Messages
	StatusMessage string
	ErrorMessage  string
}

// HistoryEntry represents a version history entry
type HistoryEntry struct {
	Version     int64
	Value       string
	Modified    string
	ValueLoaded bool // Whether the value has been fetched
}

// TreeNode represents a node in the tree view
type TreeNode struct {
	Name       string
	Path       string
	IsDir      bool
	Depth      int
	Expanded   bool
	Entry      *cache.CacheEntry // nil for directories
	ChildCount int
}

// ConfirmState represents confirmation dialog state
type ConfirmState struct {
	Active      bool
	Action      string // "delete"
	Target      string // parameter name
	ConfirmText string // text user must type
	Input       string // current input
	ErrorMsg    string
}
