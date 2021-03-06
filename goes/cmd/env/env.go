// Copyright © 2015-2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by the GPL-2 license described in the
// LICENSE file.

package env

import (
	"fmt"
	"os"
	"strings"

	"github.com/platinasystems/go/goes"
	"github.com/platinasystems/go/goes/cmd"
	"github.com/platinasystems/go/goes/lang"
)

const (
	Name    = "env"
	Apropos = "run a program in a modified environment"
	Usage   = "env [NAME[=VALUE... COMMAND [ARGS...]]]"
	Man     = `
DESCRIPTION
	Running 'env' without any arguments prints all environment
	variables.  Runnung 'env' with one argument prints the value of
	the named variable.  Running this with at least one NAME=VALUE
	argument sets each NAME to VALUE in the environment and runs
	COMMAND.`
)

var (
	apropos = lang.Alt{
		lang.EnUS: Apropos,
	}
	man = lang.Alt{
		lang.EnUS: Man,
	}
)

func New() *Command { return new(Command) }

type Command struct {
	g *goes.Goes
}

func (*Command) Apropos() lang.Alt   { return apropos }
func (c *Command) Goes(g *goes.Goes) { c.g = g }
func (*Command) Kind() cmd.Kind      { return cmd.DontFork | cmd.CantPipe }

func (c *Command) Main(args ...string) error {
	switch len(args) {
	case 0:
		for _, env := range os.Environ() {
			fmt.Println(env)
		}
	case 1:
		fmt.Println(os.Getenv(args[0]))
	default:
		for {
			eq := strings.Index(args[0], "=")
			if eq < 0 {
				break
			}
			os.Setenv(args[0][:eq], args[0][eq+1:])
			args = args[1:]
		}
		return c.g.Main(args...)
	}
	return nil
}

func (*Command) Man() lang.Alt  { return man }
func (*Command) String() string { return Name }
func (*Command) Usage() string  { return Usage }
