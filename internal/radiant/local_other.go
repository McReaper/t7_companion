//go:build !windows

package radiant

import (
	"context"
	"errors"
	"io"
)

type local struct{}

// Local is this computer: Radiant runs on Windows only.
var Local Machine = local{}

var errNotWindows = errors.New("only Windows runs Radiant")

func (local) Bridges() ([]int, error)                               { return nil, nil }
func (local) Dial(context.Context, int) (io.ReadWriteCloser, error) { return nil, errNotWindows }
func (local) Start(string, string, ...string) (int, error)          { return 0, errNotWindows }
func (local) Running(int) bool                                      { return false }
func (local) Radiants() ([]int, error)                              { return nil, nil }
func (local) Modules(int) ([]string, error)                         { return nil, errNotWindows }
func (local) Ready(context.Context, int) error                      { return errNotWindows }
func (local) Inject(int, string) error                              { return errNotWindows }
func (local) Control(int, string) (string, error)                   { return "", errNotWindows }
func (local) WakeDisplay()                                          {}
