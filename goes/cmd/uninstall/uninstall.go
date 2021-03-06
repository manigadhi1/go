// Copyright 2016-2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by the GPL-2 license described in the
// LICENSE file.

// Package uninstall provides the named command that stops and removes
// /usr/bin/goes and it's associated files.
package uninstall

import (
	"os/exec"
	"syscall"

	"github.com/platinasystems/go/goes"
	"github.com/platinasystems/go/goes/lang"
	"github.com/platinasystems/go/internal/prog"
)

const (
	Name    = "uninstall"
	Apropos = "uninstall this goes machine"
	Usage   = "uninstall"

	EtcInitdGoes       = "/etc/init.d/goes"
	EtcDefaultGoes     = "/etc/default/goes"
	BashCompletionGoes = "/usr/share/bash-completion/completions/goes"
)

var apropos = lang.Alt{
	lang.EnUS: Apropos,
}

// Machines may use this Hook to complete its removal.
var Hook = func() error { return nil }

func New() *Command { return new(Command) }

type Command struct {
	g *goes.Goes
}

func (*Command) Apropos() lang.Alt   { return apropos }
func (c *Command) Goes(g *goes.Goes) { c.g = g }

func (c *Command) Main(...string) error {
	err := c.g.Main("stop")
	if err != nil {
		return err
	}
	exec.Command("/usr/sbin/update-rc.d", "goes", "remove").Run()
	err = Hook()
	syscall.Unlink(EtcInitdGoes)
	syscall.Unlink(EtcDefaultGoes)
	syscall.Unlink(BashCompletionGoes)
	syscall.Unlink(prog.Install)
	return err
}

func (*Command) String() string { return Name }
func (*Command) Usage() string  { return Usage }
