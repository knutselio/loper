package ui

// Binding is one of loper's own keys, or a few keys that do one thing, as
// the footer, the help overlay and --help show it.
type Binding struct {
	Keys   string // as the help shows them, such as "q, ctrl-c"
	Action string // what they do, as the help says it
	// FooterKey and FooterLabel are the binding's item in the footer, such as
	// "q" and "quit"; a binding without a label is not in the footer.
	FooterKey, FooterLabel string
}

// Section is a group of bindings under a title; the keys of the list itself
// have no title.
type Section struct {
	Title    string
	Bindings []Binding
}

// KeyList returns loper's own keys, the one list the footer, the help
// overlay and --help draw on. The footer shows its items in this order.
func KeyList() []Section {
	return []Section{
		{Bindings: []Binding{
			{Keys: "j / k, ↓ / ↑", Action: "Move the selection"},
			{Keys: "g / G", Action: "Jump to the top / bottom"},
			{Keys: "ctrl-d / ctrl-u", Action: "Move half a page down / up"},
			{Keys: "enter", Action: "Open the target menu for the selected repo", FooterKey: "enter", FooterLabel: "open"},
			{Keys: "a target's key", Action: "Open the selected repo in that target"},
			{Keys: "/", Action: "Start the search", FooterKey: "/", FooterLabel: "search"},
			{Keys: "s", Action: "Switch the sort", FooterKey: "s", FooterLabel: "sort"},
			{Keys: "r", Action: "Force a full refresh", FooterKey: "r", FooterLabel: "refresh"},
			{Keys: "?", Action: "Show the help overlay", FooterKey: "?", FooterLabel: "help"},
			{Keys: "q, ctrl-c", Action: "Quit", FooterKey: "q", FooterLabel: "quit"},
			{Keys: "esc", Action: "Close a menu or dialog, or clear the search"},
		}},
		{Title: "In the target menu", Bindings: []Binding{
			{Keys: "j / k, ↓ / ↑", Action: "Move the highlight"},
			{Keys: "enter", Action: "Run the highlighted target"},
			{Keys: "a target's key", Action: "Run that target"},
			{Keys: "esc", Action: "Close the menu"},
		}},
		{Title: "While searching", Bindings: []Binding{
			{Keys: "↓ / ↑, ctrl-n / ctrl-p", Action: "Move through the matches"},
			{Keys: "enter", Action: "Open the target menu for the selected match"},
			{Keys: "esc", Action: "Clear the search"},
		}},
	}
}
