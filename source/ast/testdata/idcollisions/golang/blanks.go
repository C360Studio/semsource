// Package fixture holds Go declarations that today share one entity ID.
package fixture

import (
	"io"
	"os"
)

var _ io.Reader = (*os.File)(nil)
var _ io.Writer = (*os.File)(nil)

const (
	_ = iota
	A
	B
)

const _ = "blank"

func _() {}

func init() {}

func init() {}
