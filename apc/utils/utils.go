package utils

import (
	"fmt"

	"github.com/astronomer/astro-cli/pkg/input"
)

const (
	defaultPaginationOptions      = "f. first p. previous n. next q. quit\n> "
	paginationWithoutNextOptions  = "f. first p. previous q. quit\n> "
	paginationWithNextQuitOptions = "n. next q. quit\n> "
)

type PaginationOptions struct {
	CursorID   string
	PageSize   int
	Quit       bool
	PageNumber int
}

// PromptPaginatedOption Show pagination option based on page size and total record
func PromptPaginatedOption(previousCursorID, nextCursorID string, take, totalRecord, pageNumber int, lastPage bool) PaginationOptions {
	for {
		pageSize := Abs(take)
		gotoOptionMessage := defaultPaginationOptions
		gotoOptions := make(map[string]PaginationOptions)
		gotoOptions["f"] = PaginationOptions{CursorID: "", PageSize: pageSize, Quit: false, PageNumber: 0}
		gotoOptions["p"] = PaginationOptions{CursorID: previousCursorID, PageSize: -pageSize, Quit: false, PageNumber: pageNumber - 1}
		gotoOptions["n"] = PaginationOptions{CursorID: nextCursorID, PageSize: pageSize, Quit: false, PageNumber: pageNumber + 1}
		gotoOptions["q"] = PaginationOptions{CursorID: "", PageSize: 0, Quit: true, PageNumber: 0}

		if totalRecord < pageSize || lastPage {
			delete(gotoOptions, "n")
			gotoOptionMessage = paginationWithoutNextOptions
		}

		if pageNumber == 0 {
			delete(gotoOptions, "p")
			delete(gotoOptions, "f")
			gotoOptionMessage = paginationWithNextQuitOptions
		}

		in := input.Text("\n\nPlease select one of the following options\n" + gotoOptionMessage)
		value, found := gotoOptions[in]
		if found {
			return value
		}
		fmt.Print("\nInvalid option")
	}
}

func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
