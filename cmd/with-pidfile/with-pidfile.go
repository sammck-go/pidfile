package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	pidfile "github.com/sammck/gopidfile"
)

func run() int {
	pidFileDirFlag := flag.String("d", "", "Directory for pidfile. Defaults to /run.")
	pidFileFlag := flag.String("f", "", "pidfile name. Defaults to <prog>.pid")
	acquireTimeoutSecondsFlag := flag.Int64("t", 0, "Maximum seconds to wait for pifdile lock. Defaults to 0.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [<option>...] <cmd> [<cmd-arg>...]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Wrap a process invocation with a pidfile.\n\n")
		fmt.Fprintf(os.Stderr, "   <cmd>          A program to launch\n")
		fmt.Fprintf(os.Stderr, "   <cmd-arg>...   Zero or more arguments or options to the launched program\n\n")
		fmt.Fprintln(os.Stderr, "Options:")

		flag.PrintDefaults()
	}

	flag.Parse()

	cmdline := flag.Args()

	if len(cmdline) == 0 {
		fmt.Fprintln(os.Stderr, "with-pidfile: No command line given")
		return 1
	}

	progFileName := cmdline[0]
	progArgs := cmdline[1:]
	progBase := filepath.Base(progFileName)
	pidFile := progBase + ".pid"
	if *pidFileFlag != "" {
		pidFile = *pidFileFlag
	}

	// fmt.Fprintf(os.Stderr, "with-pidfile: ProgBase=%q, progArgs=%q, pidFile=%q, pidFileDir=%q\n", progBase, progArgs, pidFile, *pidFileDirFlag)

	cfg := pidfile.NewConfig(pidfile.WithDeferPublish())

	if *pidFileDirFlag != "" {
		cfg = cfg.Refine(pidfile.WithDirName(*pidFileDirFlag))
	}

	cfg = cfg.Refine(pidfile.WithFileName(pidFile))

	if *acquireTimeoutSecondsFlag != 0 {
		cfg = cfg.Refine(pidfile.WithAcquireTimeout(time.Duration(*acquireTimeoutSecondsFlag) * time.Second))
	}

	pidfile, err := pidfile.NewPidFile(pidfile.WithConfig(cfg))
	if err != nil {
		fmt.Fprintln(os.Stderr, "with-pidfile: Could not activate pidfile: ", err)
		return 1
	}

	defer pidfile.Close()

	// fmt.Fprintln(os.Stderr, "Successfully activated pidfile", pidfile.GetPathName())

	ctx, cancel := context.WithCancel(context.Background())

	sigintChan := make(chan os.Signal)
	signal.Notify(sigintChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigintChan
		fmt.Fprintln(os.Stderr, "with-pidfile: received SIGINT, shutting down")
		cancel()
	}()

	exitCode := 0
	cmd := exec.CommandContext(ctx, progFileName, progArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err = cmd.Start()
	if err == nil {
		pid := cmd.Process.Pid
		err = pidfile.PublishWithPid(pid)
		if err != nil {
			fmt.Fprintln(os.Stderr, "with-pidfile: Could not publish pidfile: ", err)
			return 1
		}
		// fmt.Fprintln(os.Stderr, "Successfully published pidfile", pidfile.GetPathName(), "with PID", pidfile.GetPid())
	} else {
		fmt.Fprintf(os.Stderr, "with-pidfile: Unable to launch %s: %s\n", progFileName, err)
		return 1
	}

	procDoneChan := make(chan error)
	go func() {
		err := cmd.Wait()
		procDoneChan <- err
	}()

	err = cmd.Wait()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			fmt.Fprintf(os.Stderr, "with-pidfile: Unable to complete child process %s: %s\n", progFileName, err)
			return 1
		}
		exitCode = exitErr.ExitCode()
		// fmt.Fprintln(os.Stderr, "with-pidfile: process exited with code ", exitCode)
	}

	err = pidfile.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "with-pidfile: Could not close pidfile: ", err)
		return 1
	}

	// fmt.Fprintln(os.Stderr, "Successfully closed pidfile", pidfile.GetPathName())

	return exitCode
}

func main() {
	exitCode := run()
	os.Exit(exitCode)
}
