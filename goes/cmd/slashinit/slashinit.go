// Copyright © 2015-2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by the GPL-2 license described in the
// LICENSE file.

package slashinit

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cavaliercoder/grab"
	"github.com/platinasystems/go/goes"
	"github.com/platinasystems/go/goes/cmd"
	"github.com/platinasystems/go/goes/lang"
	"github.com/platinasystems/go/internal/log"
	"github.com/platinasystems/go/internal/url"
)

const (
	Name    = "/init"
	Apropos = "bootstrap"
	Usage   = "init"
	Man     = `
DESCRIPTION
	The '/init' command that mounts and pivots to the 'goesroot' kernel
	parameter before executing its '/sbin/init'.  The machine may reassign
	the Hook closure to perform target specific tasks prior to the
	'goesroot' pivot. The kernel command may include 'goes=overwrite' to
	force copy of '/bin/goes' from the initrd to the named 'goesroot'.

	If the target root is not mountable, the 'goesinstaller' parameter
	specifies an installer/recovery system to use to repair the system. The
	parameter to this is three comma-seperated URLs. The first is
	mandatory, and is the kernel to load. The second is the optional
	initramfs to load. The third is the optional FDT to load. The kernel is
	loaded via the kexec command.`

	zero = uintptr(0)
)

var (
	Hook    = func() error { return nil }
	apropos = lang.Alt{
		lang.EnUS: Apropos,
	}
	man = lang.Alt{
		lang.EnUS: Man,
	}
)

func New() *Command { return new(Command) }

func init() {
	if os.Getpid() != 1 {
		return
	}
	if os.Args[0] != Name {
		return
	}

	for _, mnt := range []struct {
		dir    string
		dev    string
		fstype string
		mode   os.FileMode
	}{
		{"/dev", "devtmpfs", "devtmpfs", 0755},
		{"/dev/pts", "devpts", "devpts", 0755},
		{"/proc", "proc", "proc", 0555},
		{"/sys", "sysfs", "sysfs", 0555},
		{"/run", "tmpfs", "tmpfs", 0755},
	} {
		if _, err := os.Stat(mnt.dir); os.IsNotExist(err) {
			err = os.Mkdir(mnt.dir, os.FileMode(mnt.mode))
			if err != nil {
				log.Print("err", mnt.dir, ": ", err)
				continue
			}
		}
		err := syscall.Mount(mnt.dev, mnt.dir, mnt.fstype, zero, "")
		if err != nil {
			log.Print("err", mnt.dir, ": ", err)
		}
	}
	for _, ln := range []struct {
		src, dst string
	}{
		{"../proc/self/fd/0", "/dev/stdin"},
		{"../proc/self/fd/1", "/dev/stdout"},
		{"../proc/self/fd/2", "/dev/stderr"},
	} {
		if _, err := os.Stat(ln.dst); os.IsNotExist(err) {
			err = os.Symlink(ln.src, ln.dst)
			if err != nil {
				log.Print("err", "ln", ln.dst, "->", ln.src,
					":", err)
			}
		}
	}
	consoles, err := ioutil.ReadFile("/sys/devices/virtual/tty/console/active")
	if err != nil {
		log.Print("err", "open", "/sys/devices/virtual/tty/console/active", err)
	} else {
		consoleList := strings.Fields(string(consoles))
		console := "/dev/" + consoleList[len(consoleList)-1]
		for fd := 0; fd <= 2; fd++ {
			err := syscall.Close(fd)
			if err != nil {
				log.Print("err", "console close", ":", err)
			}
			_, err = syscall.Open(console, syscall.O_RDWR, 0)
			if err != nil {
				log.Print("err", "console reopen", ":", err)
			}
		}
	}

}

type Command struct {
	g *goes.Goes
}

func (*Command) Apropos() lang.Alt   { return apropos }
func (c *Command) Goes(g *goes.Goes) { c.g = g }
func (*Command) Kind() cmd.Kind      { return cmd.DontFork }

func (c *Command) Main(_ ...string) error {
	goesRoot := filepath.SplitList(os.Getenv("goesroot"))
	goesinstaller := os.Getenv("goesinstaller")
	goesInitDelay := os.Getenv("goesinitdelay")
	if goesInitDelay != "" {
		i, err := strconv.Atoi(goesInitDelay)
		if err != nil {
			fmt.Printf("goesInitDelay parse error: %s\n", err)
		} else {
			time.Sleep(time.Duration(i) * time.Second)
		}
	}
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(r)
			}
			c.emergencyShell()
		}()
		if r := recover(); r != nil {
			fmt.Println(r)
		}
		if len(goesinstaller) > 0 {
			params := strings.Split(goesinstaller, ",")
			err := c.installer(params)
			if err != nil {
				log.Print("err", "installer", params[0],
					":", err)
			}
		}
	}()
	err := Hook()
	if err != nil {
		panic(fmt.Errorf("Error from board hook: ", err))
	}
	var root, script string
	if len(goesRoot) >= 1 && len(goesRoot[0]) > 0 {
		root = goesRoot[0]
	}
	if len(goesRoot) >= 2 && len(goesRoot[1]) > 0 {
		script = goesRoot[1]
	}

	if len(root) > 0 {
		c.pivotRoot("/newroot", root, script)
	}
	c.makeTargetDirs()
	c.makeTargetLinks()
	c.mountTargetVirtualFilesystems()
	c.runSbinInit()
	err = c.g.Main("start")

	return err
}

func (*Command) Man() lang.Alt  { return man }
func (*Command) String() string { return Name }
func (*Command) Usage() string  { return Usage }

func (*Command) makeRootDirs(mountPoint string) {
	for _, dir := range []struct {
		name string
		mode os.FileMode
	}{
		{"/bin", 0775},
		{"/sbin", 0755},
		{"/usr", 0755},
		{"/usr/bin", 0755},
	} {
		if _, err := os.Stat(mountPoint + dir.name); os.IsNotExist(err) {
			err := os.Mkdir(mountPoint+dir.name, dir.mode)
			if err != nil {
				panic(fmt.Errorf("mkdir %s: %s",
					mountPoint+dir.name, err))
			}
		}
	}
}

func (*Command) makeRootFiles(mountPoint string) {
	for _, cp := range []struct {
		src, dst string
	}{
		{"/init", "/usr/bin/goes"},
		{"/usr/bin/gdbserver", "/usr/bin/gdbserver"},
	} {
		_, err := os.Stat(mountPoint + cp.dst)
		if os.IsNotExist(err) ||
			os.Getenv("goes") == "overwrite" {
			r, err := os.Open(cp.src)
			if err != nil {
				panic(fmt.Errorf("open %s: %s", cp.src, err))
			}
			defer r.Close()
			w, err := os.Create(mountPoint + cp.dst)
			if err != nil {
				panic(fmt.Errorf("create %s: %s",
					mountPoint+cp.dst, err))
			}
			defer w.Close()
			_, err = io.Copy(w, r)
			if err != nil {
				panic(fmt.Errorf("copy %s to %s: %s",
					cp.src, mountPoint+cp.dst, err))
			}
			if err = os.Chmod(cp.dst, 0755); err != nil {
				panic(fmt.Errorf("chmod %s: %s",
					mountPoint+cp.dst, err))
			}
		}
	}
}

func (*Command) makeRootLinks(mountPoint string) {
	for _, ln := range []struct {
		src, dst string
	}{
		{"../usr/bin/goes", "/sbin/init"},
	} {
		if _, err := os.Stat(mountPoint + ln.dst); os.IsNotExist(err) {
			err = os.Symlink(ln.src, mountPoint+ln.dst)
			if err != nil {
				panic(fmt.Errorf("ln %s->%s: %s", ln.src,
					mountPoint+ln.dst, err))
			}
		}
	}
}

func (*Command) moveVirtualFileSystems(mountPoint string) {
	for _, mv := range []struct {
		src  string
		dst  string
		mode os.FileMode
	}{
		{"/run", "/run", 0755},
		{"/sys", "/sysfs", 0555},
		{"/proc", "/proc", 0555},
		{"/dev", "/dev", 0755},
	} {
		if _, err := os.Stat(mountPoint + mv.dst); os.IsNotExist(err) {
			err = os.Mkdir(mountPoint+mv.dst, os.FileMode(mv.mode))
			if err != nil {
				panic(fmt.Errorf("mkdir %s: %s",
					mountPoint+mv.dst, err))
			}
		}
		err := syscall.Mount(mv.src, mountPoint+mv.dst, "",
			syscall.MS_MOVE, "")
		if err != nil {
			panic(fmt.Errorf("mount -o move %s %s: %s",
				mv.src, mountPoint+mv.dst, err))
		}
	}
}

func (*Command) unlinkRootFiles() {
	for _, fn := range []string{
		"/usr/bin/gdbserver",
		"/init",
		"/bin/goes",
	} {
		syscall.Unlink(fn)
	}
}

func (*Command) rmdirRootDirs() {
	for _, dir := range []string{
		"/run",
		"/sys",
		"/proc",
		"/dev",
		"/usr/bin",
		"/usr",
		"/bin",
	} {
		syscall.Rmdir(dir)
	}
}

func (*Command) makeTargetDirs() {
	for _, dir := range []struct {
		name string
		mode os.FileMode
	}{
		{"/root", 0700},
		{"/tmp", 01777},
		{"/var", 0755},
	} {
		if _, err := os.Stat(dir.name); os.IsNotExist(err) {
			err = os.Mkdir(dir.name, dir.mode)
			if err != nil {
				panic(fmt.Errorf("mkdir %s: %s", dir.name, err))
			}
		}
	}
}

func (*Command) makeTargetLinks() {
	for _, ln := range []struct {
		src, dst string
	}{
		{"../run", "/var/run"},
	} {
		if _, err := os.Stat(ln.dst); os.IsNotExist(err) {
			err = os.Symlink(ln.src, ln.dst)
			if err != nil {
				panic(fmt.Errorf("ln %s -> %s: %s",
					ln.dst, ln.src, err))
			}
		}
	}
}

func (*Command) mountTargetVirtualFilesystems() {
	for _, mnt := range []struct {
		dir    string
		dev    string
		fstype string
	}{
		{"/tmp", "tmpfs", "tmpfs"},
	} {
		err := syscall.Mount(mnt.dev, mnt.dir, mnt.fstype, zero, "")
		if err != nil {
			panic(fmt.Errorf("mount -t %s %s %s: %s",
				mnt.fstype, mnt.dev, mnt.dir, err))
		}
	}
}

func (c *Command) pivotRoot(mountPoint string, root string, script string) {
	_, err := os.Stat(mountPoint)
	if os.IsNotExist(err) {
		err = os.Mkdir(mountPoint, os.FileMode(0755))
		if err != nil {
			panic(fmt.Errorf("Error creating %s: %s",
				mountPoint, err))
		}
	}

	if root == "auto" {
		err := c.g.Main("mount", "-p", "-F", mountPoint)
		if err != nil {
			panic(fmt.Errorf("Error in automount: %v", err))
		}
		err = c.g.Main("resize")
		bootCmd := []string{"boot"}
		if script != "" {
			bootCmd = append(bootCmd, "-t")
			bootCmd = append(bootCmd, script)
		}
		dirs, err := ioutil.ReadDir(mountPoint)
		if err != nil {
			panic(fmt.Errorf("Error reading automount dir: %v",
				err))
		}
		for _, dir := range dirs {
			bootCmd = append(bootCmd, mountPoint+"/"+dir.Name()+
				":root=/dev/"+dir.Name())
			bootCmd = append(bootCmd, mountPoint+"/"+dir.Name()+
				"/boot:root=/dev/"+dir.Name())
			bootCmd = append(bootCmd, mountPoint+"/"+dir.Name()+
				"/d-i:root=/dev/"+dir.Name())
		}
		err = c.g.Main(bootCmd...)
		if err == nil {
			panic(fmt.Errorf("Aborted"))
		} else {
			panic(fmt.Errorf("Error in autoboot: %v", err))
		}
	}

	err = c.g.Main("mount", root, mountPoint)
	if err != nil {
		panic(fmt.Errorf("Error mounting %s on %s: %s",
			root, mountPoint, err))
	}

	if len(script) > 0 {
		err := c.g.Main("source", script)
		if err != nil {
			const format = "Error running boot script %s on %s: %s"
			panic(fmt.Errorf(format, script, root, err))
		}
	}
	c.makeRootDirs(mountPoint)
	c.makeRootFiles(mountPoint)
	c.makeRootLinks(mountPoint)
	c.moveVirtualFileSystems(mountPoint)

	if err = os.Chdir(mountPoint); err != nil {
		panic(fmt.Errorf("chdir %s: %s", mountPoint, err))
	}
	c.unlinkRootFiles()
	c.rmdirRootDirs()
	err = syscall.Mount(mountPoint, "/", "", syscall.MS_MOVE, "")
	if err != nil {
		panic(fmt.Errorf("mount %s /: %s", mountPoint, err))
	}
	if err = syscall.Chroot("."); err != nil {
		panic(fmt.Errorf("chroot .:%s", err))
	}
}

func (*Command) runSbinInit() {
	if err := os.Setenv("PATH", "/bin:/usr/bin"); err != nil {
		panic(fmt.Errorf("Setenv PATH: %s", err))
	}
	if err := os.Setenv("SHELL", "/bin/goes"); err != nil {
		panic(fmt.Errorf("Setenv SHELL: %s", err))
	}
	if err := os.Chdir("/root"); err != nil {
		panic(fmt.Errorf("chdir /root: %s", err))
	}
	if err := os.Setenv("HOME", "/root"); err != nil {
		panic(fmt.Errorf("Setenv HOME: %s", err))
	}
	if len(os.Getenv("TERM")) == 0 {
		if err := os.Setenv("TERM", "linux"); err != nil {
			panic(fmt.Errorf("Setenv TERM: %s", err))
		}
	}
	const sbininit = "/sbin/init"
	if _, err := os.Stat(sbininit); err != nil {
		if os.IsNotExist(err) {
			return
		}
		panic(fmt.Errorf("stat %s: %s", sbininit, err))
	}

	if err := syscall.Exec(sbininit, []string{sbininit}, []string{
		"PATH=" + os.Getenv("PATH"),
		"SHELL=" + os.Getenv("SHELL"),
		"HOME=" + os.Getenv("HOME"),
		"TERM=" + os.Getenv("TERM"),
	}); err != nil {
		panic(fmt.Errorf("exec %s: %s", sbininit, err))
	}
}

func (c *Command) emergencyShell() {
	for {
		fmt.Println("Dropping into emergency goes shell...\n")
		err := c.g.Main("cli")
		if err != nil && err != io.EOF {
			fmt.Println(err)
		}
	}
}

func (c *Command) installer(params []string) error {
	if len(params) < 1 || len(params[0]) == 0 {
		return fmt.Errorf("KERNEL: missing")
	}

	reqs := make([]*grab.Request, 0)

	req, err := grab.NewRequest(params[0])
	if err != nil {
		return err
	}
	req.Filename = "kernel"
	reqs = append(reqs, req)

	if len(params) >= 2 && len(params[1]) > 0 {
		req, err := grab.NewRequest(params[1])
		if err != nil {
			return err
		}
		req.Filename = "initramfs"
		reqs = append(reqs, req)
	}

	if len(params) >= 3 && len(params[2]) > 0 {
		req, err := grab.NewRequest(params[2])
		if err != nil {
			return err
		}
		req.Filename = "fdt"
		reqs = append(reqs, req)
	}

	successes, err := url.FetchReqs(0, reqs)
	if err != nil {
		return err
	}

	if successes == len(reqs) {
		fmt.Printf("All files loaded successfully!")
	}

	return c.g.Main("kexec", "-e",
		"-k", "kernel",
		"-i", "initramfs",
		"-c", "console=ttyS0,115200")
}
