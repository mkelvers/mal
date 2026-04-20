package shared

// TabClass returns the CSS class for watchlist filter tabs
func TabClass(active bool) string {
	base := "shrink-0 whitespace-nowrap bg-(--panel-soft) px-2 py-1 text-xs text-(--text-muted) no-underline hover:bg-(--surface-tab-hover) hover:text-(--text) hover:no-underline"
	if active {
		return "shrink-0 whitespace-nowrap bg-(--surface-tab-active) px-2 py-1 text-xs text-(--accent) no-underline hover:no-underline"
	}
	return base
}
