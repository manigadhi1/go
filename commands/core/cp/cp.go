// Copyright 2015-2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license described in the
// LICENSE file.

package cp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/platinasystems/go/flags"
	"github.com/platinasystems/go/parms"
	"github.com/platinasystems/go/url"
)

const Name = "cp"

type cmd struct{}

func New() cmd { return cmd{} }

func (cmd) String() string { return Name }

func (cmd) Usage() string {
	return `cp [-v] -T SOURCE DESTINATION
	cp [-v] -t DIRECTORY SOURCE...
	cp [-v] SOURCE... DIRECTORY`
}

func (cmd) Main(args ...string) error {
	cp := func(source, dest string, verbose bool) error {
		r, err := url.Open(source)
		if err != nil {
			return err
		}
		defer r.Close()
		w, err := url.Create(dest)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, r)
		if err != nil {
			return err
		}
		if verbose {
			fmt.Println("Copied", source, "to", dest)
		}
		return nil
	}
	valid := func(dir string) error {
		fi, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%s: isn't a directory", dir)
		}
		return nil
	}

	flag, args := flags.New(args, "-T", "-v")
	parm, args := parms.New(args, "-t")

	if flag["-T"] {
		switch len(args) {
		case 0:
			return fmt.Errorf("SOURCE DESTINATION: missing")
		case 1:
			return fmt.Errorf("DESTINATION: missing")
		case 2:
			return cp(args[0], args[1], flag["-v"])
		default:
			return fmt.Errorf("%s :unexpected", args[2:])
		}
	} else if dir := parm["-t"]; len(dir) > 0 {
		if len(args) == 0 {
			return fmt.Errorf("SOURCE: missing")
		}
		if err := valid(dir); err != nil {
			return err
		}
		for _, source := range args {
			dest := filepath.Join(dir, filepath.Base(source))
			return cp(source, dest, flag["-v"])
		}
	} else {
		switch len(args) {
		case 0:
			return fmt.Errorf("SOURCE: missing")
		case 1:
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			dest := filepath.Join(wd, filepath.Base(args[0]))
			return cp(args[0], dest, flag["-v"])
		case 2:
			return cp(args[0], args[1], flag["-v"])
		default:
			dir := args[len(args)-1]
			if err := valid(dir); err != nil {
				return err
			}
			for _, t := range args[:len(args)-1] {
				b := filepath.Base(t)
				l := filepath.Join(dir, b)
				return cp(t, l, flag["-v"])
			}
		}
	}
	return nil
}

func (cmd) Apropos() map[string]string {
	return map[string]string{
		"en_US.UTF-8": "copy files and directories",
	}
}

func (cmd) Man() map[string]string {
	return map[string]string{
		"en_US.UTF-8": `NAME
	cp - copy files and directories

SYNOPSIS
	cp [-v] [-T] SOURCE DESTINATION
	cp [-v] SOURCE... DIRECTORY
	cp [-v] -t DIRECTORY SOURCE...

DESCRIPTION
	Copy SOURCE to DEST, or multiple SOURCE(s) to DIRECTORY where
	SOURCE, DEST and DIRECTORY may all be URLs.

OPTIONS
	-v	verbose`,
	}
}
