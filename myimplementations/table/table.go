package table

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
)

// newTable returns a table with the app's standard look.
func NewTable(title string) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.SetTitle("%s", title)
	return t
}
