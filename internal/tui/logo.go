package tui

// logoLines returns the ASCII-art wordmark "YT-DLP" (5 rows). Trailing spaces
// are insignificant because the block is right-aligned in the header.
func logoLines() []string {
	return []string{
		`__   __  _____        ____    _       ____`,
		`\ \ / / |_   _|      |  _ \  | |     |  _ \`,
		` \ V /    | |    ___ | | | | | |     | |_) |`,
		`  | |     | |        | |_| | | |___  |  __/`,
		`  |_|     |_|        |____/  |_____| |_|`,
	}
}

// logoWidth is the rendered width of the wordmark in cells.
func logoWidth() int { return 44 }
